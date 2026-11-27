package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"docklift/internal/db"
	"docklift/internal/js"
	"docklift/internal/store"
)

// KeepScanRows is how many recent successful deployments are scanned for
// image tags. Stop and restart rows are "success" too but carry no tags,
// so the scan must look past them.
const KeepScanRows = 100

// ImageTags is a deployment's service -> image tag map (the image_tags Json
// column). It keeps JavaScript object key order — array-index keys ("0",
// "12") ascending first, then the others in insertion order — so stored
// JSON and iteration order match the TS backend.
type ImageTags struct {
	keys []string
	tags map[string]string
}

// Set adds or replaces the tag of a service.
func (m *ImageTags) Set(service, tag string) {
	if m.tags == nil {
		m.tags = map[string]string{}
	}
	if _, ok := m.tags[service]; !ok {
		m.keys = append(m.keys, service)
	}
	m.tags[service] = tag
}

// Get returns the tag of a service.
func (m ImageTags) Get(service string) (string, bool) {
	t, ok := m.tags[service]
	return t, ok
}

// Len is the number of services.
func (m ImageTags) Len() int { return len(m.keys) }

// Services returns the service names in JavaScript object key order.
func (m ImageTags) Services() []string {
	var index, other []string
	for _, k := range m.keys {
		if isArrayIndex(k) {
			index = append(index, k)
		} else {
			other = append(other, k)
		}
	}
	slices.SortFunc(index, func(a, b string) int {
		x, _ := strconv.ParseUint(a, 10, 32)
		y, _ := strconv.ParseUint(b, 10, 32)
		return int(x) - int(y)
	})
	return append(index, other...)
}

// Values returns the tags in key order.
func (m ImageTags) Values() []string {
	out := make([]string, 0, len(m.keys))
	for _, k := range m.Services() {
		out = append(out, m.tags[k])
	}
	return out
}

// isArrayIndex reports whether a property key is a canonical array index
// (0 .. 2^32-2), which JavaScript orders before string keys.
func isArrayIndex(k string) bool {
	if k == "" || (len(k) > 1 && k[0] == '0') {
		return false
	}
	n, err := strconv.ParseUint(k, 10, 32)
	return err == nil && n < 1<<32-1
}

// MarshalJSON writes the map as a JSON object in key order, without HTML
// escaping (like JSON.stringify).
func (m ImageTags) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range m.Services() {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(jsonString(k))
		buf.WriteByte(':')
		buf.Write(jsonString(m.tags[k]))
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func jsonString(s string) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return bytes.TrimSuffix(b.Bytes(), []byte{'\n'})
}

// JSON returns the value for the image_tags column.
func (m ImageTags) JSON() *db.JSON {
	b, _ := m.MarshalJSON()
	j := db.JSON(b)
	return &j
}

// ParseImageTags reads an image_tags column (parseImageTagsJson): only a JSON
// object counts, only non-empty string values are kept, and an object with
// none is "no tags" (ok=false). A later duplicate key wins, like JSON.parse.
func ParseImageTags(raw *db.JSON) (ImageTags, bool) {
	var m ImageTags
	if raw == nil || len(*raw) == 0 {
		return m, false
	}
	dec := json.NewDecoder(bytes.NewReader(*raw))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return m, false
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return ImageTags{}, false
		}
		key, _ := kt.(string)
		var v any
		if err := dec.Decode(&v); err != nil {
			return ImageTags{}, false
		}
		if s, ok := v.(string); ok && s != "" {
			m.Set(key, s)
		} else if _, exists := m.tags[key]; exists {
			// A later non-string duplicate replaces the earlier string value.
			delete(m.tags, key)
			m.keys = slices.DeleteFunc(m.keys, func(k string) bool { return k == key })
		}
	}
	return m, m.Len() > 0
}

// ProjectID8 is the first 8 characters of the project id (dashes kept).
func ProjectID8(projectID string) string { return js.Slice(projectID, 8) }

// DockliftImageRepo is docklift-<project8>-<service slug>.
func DockliftImageRepo(projectID, serviceName string) string {
	return "docklift-" + ProjectID8(projectID) + "-" + Slug(serviceName)
}

// DockliftImageTag is the tag of a service image built by a deployment:
// docklift-<project8>-<service slug>:<deployment8>.
func DockliftImageTag(projectID, serviceName, deploymentID string) string {
	return DockliftImageRepo(projectID, serviceName) + ":" + js.Slice(deploymentID, 8)
}

var dockliftAppImageRe = regexp.MustCompile(`(?i)^docklift-[a-f0-9]{8}-[a-z0-9][a-z0-9_-]*$`)

// IsDockliftAppImage reports whether ref names a Docklift-built app image
// (never upstream postgres/nginx or the platform's own images).
func IsDockliftAppImage(ref string) bool {
	name, _, _ := strings.Cut(ref, ":")
	return dockliftAppImageRe.MatchString(name)
}

// BuildKeepImageSet is the set of Docklift app image tags in the given maps
// plus fallback (the current deploy, when not yet persisted).
func BuildKeepImageSet(maps []ImageTags, fallback *ImageTags) map[string]bool {
	keep := map[string]bool{}
	add := func(m ImageTags) {
		for _, tag := range m.Values() {
			if tag != "" && IsDockliftAppImage(tag) {
				keep[tag] = true
			}
		}
	}
	for _, m := range maps {
		add(m)
	}
	if fallback != nil {
		add(*fallback)
	}
	return keep
}

// PickTaggedImageMaps returns the first limit image-tag maps of rows ordered
// newest first, skipping rows without tags so they never use up a keep slot.
func PickTaggedImageMaps(rows []*db.JSON, limit int) []ImageTags {
	var maps []ImageTags
	for _, raw := range rows {
		if len(maps) >= limit {
			break
		}
		if m, ok := ParseImageTags(raw); ok {
			maps = append(maps, m)
		}
	}
	return maps
}

// RollbackGuard is the verdict of RollbackTargetGuard: OK, or the HTTP
// status and error message to answer.
type RollbackGuard struct {
	OK     bool
	Status int
	Error  string
}

// RollbackTargetGuard checks that a deployment can be restored ("Restore
// previous"): it must be a success, not the current success, and have
// stored image tags.
func RollbackTargetGuard(targetID, latestSuccessID, status string, tags ImageTags) RollbackGuard {
	switch {
	case status != StatusSuccess:
		return RollbackGuard{Status: 404, Error: "Target successful deployment not found"}
	case latestSuccessID != "" && latestSuccessID == targetID:
		return RollbackGuard{Status: 400, Error: "That deployment is already the current success"}
	case tags.Len() == 0:
		return RollbackGuard{Status: 409, Error: "This deployment has no stored image tags (pre-keep-2). Redeploy from git instead."}
	}
	return RollbackGuard{OK: true}
}

// ListedImage is a local Docklift app image.
type ListedImage struct {
	ID   string
	Tags []string
}

// ListDockliftImages lists local images tagged docklift-<project8>-* (or any
// docklift-* app image when projectID is "").
func (r *Runtime) ListDockliftImages(ctx context.Context, projectID string) ([]ListedImage, error) {
	images, err := r.Docker.ListImages(ctx)
	if err != nil {
		return nil, err
	}
	prefix := "docklift-"
	if projectID != "" {
		prefix = "docklift-" + ProjectID8(projectID) + "-"
	}
	var out []ListedImage
	for _, img := range images {
		var tags []string
		for _, t := range img.Tags {
			if strings.HasPrefix(t, prefix) && IsDockliftAppImage(t) {
				tags = append(tags, t)
			}
		}
		if len(tags) > 0 {
			out = append(out, ListedImage{ID: img.ID, Tags: tags})
		}
	}
	return out, nil
}

// CleanupResult reports an image cleanup.
type CleanupResult struct {
	Kept         []string
	Removed      []string
	SkippedInUse []string
	Errors       []string
}

// RemoveUnusedDockliftImages removes Docklift image tags of the project (all
// projects when projectID is "") that are neither in keep nor referenced by
// inUse (tags, image ids with or without "sha256:", or id prefixes). It never
// touches non-Docklift images.
func (r *Runtime) RemoveUnusedDockliftImages(ctx context.Context, projectID string, keep, inUse map[string]bool) (CleanupResult, error) {
	res := CleanupResult{Kept: sortedKeys(keep), Removed: []string{}, SkippedInUse: []string{}, Errors: []string{}}
	images, err := r.ListDockliftImages(ctx, projectID)
	if err != nil {
		return res, err
	}
	for _, img := range images {
		for _, tag := range img.Tags {
			if keep[tag] {
				continue
			}
			if imageInUse(tag, img.ID, inUse) {
				res.SkippedInUse = append(res.SkippedInUse, tag)
				continue
			}
			removed, err := r.Docker.RemoveImage(ctx, tag)
			if err != nil {
				res.Errors = append(res.Errors, tag+": "+err.Error())
				continue
			}
			if removed {
				res.Removed = append(res.Removed, tag)
			}
		}
	}
	return res, nil
}

func imageInUse(tag, id string, inUse map[string]bool) bool {
	bare := strings.TrimPrefix(id, "sha256:")
	if inUse[tag] || inUse[id] || inUse[bare] {
		return true
	}
	for ref := range inUse {
		if strings.TrimPrefix(ref, "sha256:") == bare || (strings.HasPrefix(ref, "sha256:") && strings.HasPrefix(id, ref)) {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// ProjectKeepSet is the keep-2 set of a project: the tags of its two newest
// tagged successful deployments.
func (r *Runtime) ProjectKeepSet(ctx context.Context, projectID string, fallback *ImageTags) (map[string]bool, error) {
	rows, err := r.Store.ListDeploymentImageTags(ctx, projectID, KeepScanRows)
	if err != nil {
		return nil, err
	}
	return BuildKeepImageSet(PickTaggedImageMaps(rows, 2), fallback), nil
}

// ContainerImageRefs returns the image refs used by the containers of the
// given services (unknown containers are skipped).
func (r *Runtime) ContainerImageRefs(ctx context.Context, services []*store.Service) map[string]bool {
	refs := map[string]bool{}
	for _, svc := range services {
		if svc.ContainerName == nil || *svc.ContainerName == "" {
			continue
		}
		if ref := r.Docker.ContainerImageRef(ctx, *svc.ContainerName); ref != "" {
			refs[ref] = true
		}
	}
	return refs
}

// PurgeResult is the summary line of a post-deploy cleanup.
type PurgeResult struct {
	Success bool
	Message string
}

// PostDeployCleanup runs after a deployment was committed as success
// (runPostDeploymentPurge): it keeps the current and previous successful
// Docklift tags of the project, removes older unused tags, and prunes unused
// BuildKit cache, writing progress to logw. It soft-fails and never touches
// non-Docklift images.
func (r *Runtime) PostDeployCleanup(ctx context.Context, projectID string, current ImageTags, logw io.Writer) PurgeResult {
	logf := func(format string, a ...any) { _, _ = fmt.Fprintf(logw, format, a...) }
	fail := func(err error) PurgeResult {
		logf("   ! Cleanup warning: %s\n", err.Error())
		return PurgeResult{Message: "! Cleanup warning: " + err.Error()}
	}
	hasAppImages := current.Len() > 0
	removed, kept := 0, 0
	if hasAppImages {
		keep, err := r.ProjectKeepSet(ctx, projectID, &current)
		if err != nil {
			return fail(err)
		}
		services, err := r.Store.ListServices(ctx, store.ServiceFilter{ProjectID: &projectID}, store.OrderNone)
		if err != nil {
			return fail(err)
		}
		inUse := r.ContainerImageRefs(ctx, services)
		for tag := range keep {
			inUse[tag] = true
		}
		result, err := r.RemoveUnusedDockliftImages(ctx, projectID, keep, inUse)
		if err != nil {
			return fail(err)
		}
		removed, kept = len(result.Removed), len(result.Kept)
		logf("   Kept %d image tag(s) (current + previous)\n", kept)
		if len(result.Removed) > 0 {
			logf("   Removed %d older unused tag(s)\n", len(result.Removed))
			for _, tag := range result.Removed[:min(8, len(result.Removed))] {
				logf("     - %s\n", tag)
			}
			if len(result.Removed) > 8 {
				logf("     … and %d more\n", len(result.Removed)-8)
			}
		} else {
			logf("   No older Docklift tags to remove\n")
		}
		if len(result.SkippedInUse) > 0 {
			logf("   Skipped %d in-use tag(s)\n", len(result.SkippedInUse))
		}
		for _, e := range result.Errors {
			logf("   ! %s\n", e)
		}
	} else {
		logf("   ○ No Docklift app images to clean (image prune skipped)\n")
	}

	bk := r.Docker.BuilderPrune(ctx, false)
	if bk.OK {
		logf("   BuildKit unused cache pruned\n")
	} else {
		out := bk.Output
		if out == "" {
			out = "failed"
		}
		logf("   ! BuildKit prune warning: %s\n", out)
	}
	if hasAppImages {
		return PurgeResult{Success: true, Message: fmt.Sprintf("✓ Keep-2 cleanup: removed %d, kept %d", removed, kept)}
	}
	return PurgeResult{Success: true, Message: "✓ BuildKit unused cache pruned (no app images)"}
}

// GlobalKeepSet is the union of every project's keep-2 set (system purge).
func (r *Runtime) GlobalKeepSet(ctx context.Context) (map[string]bool, error) {
	projects, err := r.Store.ListProjects(ctx, store.ProjectFilter{}, store.OrderNone)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	for _, p := range projects {
		set, err := r.ProjectKeepSet(ctx, p.ID, nil)
		if err != nil {
			return nil, err
		}
		for tag := range set {
			keep[tag] = true
		}
	}
	return keep, nil
}
