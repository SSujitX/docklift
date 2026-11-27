package system

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/moby/moby/client"

	"docklift/internal/api/logs"
	"docklift/internal/httpx"
	"docklift/internal/store"
)

// ImageCleanup is the part of image cleanup that POST /purge needs. It
// is implemented by the image-retention code of internal/deploy (owned by
// another package) and injected through Options.Images. Errors must wrap the
// Docker client error (%w) so an unreachable daemon is recognised.
type ImageCleanup interface {
	// KeepTags is the keep-2 set of one project:
	// buildKeepImageSet(pickTaggedImageMaps(<newest KEEP_SCAN_ROWS successful
	// deployments' image_tags>, 2)).
	KeepTags(ctx context.Context, projectID string) ([]string, error)
	// ContainerImageRef is getContainerImageRef: the image reference a
	// container was created from, "" when unknown or on any error.
	ContainerImageRef(ctx context.Context, containerName string) string
	// RemoveUnusedImages is removeUnusedDockliftImages without a project id
	// (every docklift-* image): tags outside keep and not in inUse are
	// force-removed.
	RemoveUnusedImages(ctx context.Context, keep, inUse map[string]bool) (CleanupResult, error)
	// PruneBuildKitAll is pruneBuildKitAll (`docker builder prune -af`).
	PruneBuildKitAll(ctx context.Context) (ok bool, output string)
}

// CleanupResult is the outcome of an image cleanup.
type CleanupResult struct {
	Kept         []string
	Removed      []string
	SkippedInUse []string
	Errors       []string
}

var errImageCleanupNotConfigured = errors.New("image cleanup is not configured")

type busyBody struct {
	Error        string `json:"error"`
	DeploymentID string `json:"deploymentId"`
	ProjectID    string `json:"projectId"`
}

type purgeBody struct {
	Message       string   `json:"message"`
	Details       []string `json:"details"`
	RemovedImages int      `json:"removedImages"`
	KeptImages    int      `json:"keptImages"`
	MemoryBefore  *float64 `json:"memoryBefore"`
	MemoryAfter   *float64 `json:"memoryAfter"`
	MemorySaved   *string  `json:"memorySaved"`
}

func (h *handlers) inProgressDeployment(ctx context.Context) (*store.Deployment, error) {
	status := "in_progress"
	d, err := h.d.Store.FindDeployment(ctx, store.DeploymentFilter{Status: &status}, store.OrderNone)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	return d, err
}

// POST /purge — DockLift-scoped cleanup only: unused docklift-* images
// outside each project's keep-2 set, then the whole BuildKit cache. Never a
// host-wide prune, foreign images, volumes or OS maintenance.
func (h *handlers) purge(w http.ResponseWriter, r *http.Request) {
	ok, err := h.d.Auth.RequireStepUpPassword(w, r)
	if err != nil {
		h.purgeFailed(w, err)
		return
	}
	if !ok {
		return
	}
	h.d.Log.Info("[AUDIT] System purge (DockLift-scoped) from IP: " + httpx.ClientIP(r))
	ctx := r.Context()
	var results []string
	memoryBefore, _ := h.stats.MemoryPercent(ctx)

	if busy, err := h.inProgressDeployment(ctx); err != nil {
		h.purgeFailed(w, err)
		return
	} else if busy != nil {
		httpx.JSON(w, http.StatusConflict, busyBody{
			Error:        "A deployment is in progress. Wait for it to finish before purging (avoids deleting in-flight images / BuildKit layers).",
			DeploymentID: busy.ID, ProjectID: busy.ProjectID,
		})
		return
	}
	if h.images == nil {
		h.purgeFailed(w, errImageCleanupNotConfigured)
		return
	}
	projects, err := h.d.Store.ListProjects(ctx, store.ProjectFilter{}, store.OrderNone)
	if err != nil {
		h.purgeFailed(w, err)
		return
	}
	keep := map[string]bool{}
	for _, p := range projects {
		tags, err := h.images.KeepTags(ctx, p.ID)
		if err != nil {
			h.purgeFailed(w, err)
			return
		}
		for _, t := range tags {
			keep[t] = true
		}
	}

	// Fail closed if a deploy started after the first busy check.
	if raced, err := h.inProgressDeployment(ctx); err != nil {
		h.purgeFailed(w, err)
		return
	} else if raced != nil {
		httpx.JSON(w, http.StatusConflict, busyBody{
			Error:        "A deployment started during purge preparation. Wait for it to finish before purging.",
			DeploymentID: raced.ID, ProjectID: raced.ProjectID,
		})
		return
	}

	services, err := h.d.Store.ListServices(ctx, store.ServiceFilter{}, store.OrderNone)
	if err != nil {
		h.purgeFailed(w, err)
		return
	}
	inUse := make(map[string]bool, len(keep))
	for t := range keep {
		inUse[t] = true
	}
	for _, svc := range services {
		if svc.ContainerName == nil || *svc.ContainerName == "" {
			continue
		}
		if ref := h.images.ContainerImageRef(ctx, *svc.ContainerName); ref != "" {
			inUse[ref] = true
		}
	}
	// Hardening over 2.x, which only protected the images of the
	// services' containers: an image any container on the host was created
	// from (running or stopped, tracked or not) is never force-removed.
	// Without a complete container list nothing is removed (fail closed).
	if err := h.addContainerImages(ctx, inUse); err != nil {
		h.purgeFailed(w, err)
		return
	}

	res, err := h.images.RemoveUnusedImages(ctx, keep, inUse)
	if err != nil {
		h.purgeFailed(w, err)
		return
	}
	results = append(results, fmt.Sprintf("✓ Removed %d unused Docklift image tag(s); kept %d", len(res.Removed), len(res.Kept)))
	if len(res.SkippedInUse) > 0 {
		results = append(results, fmt.Sprintf("○ Skipped %d in-use tag(s)", len(res.SkippedInUse)))
	}
	for i, e := range res.Errors {
		if i == 5 {
			break
		}
		results = append(results, "! "+e)
	}

	// Never wipe BuildKit under an in-flight build.
	busy, err := h.inProgressDeployment(ctx)
	if err != nil {
		h.purgeFailed(w, err)
		return
	}
	if busy != nil {
		results = append(results, "○ Skipped BuildKit wipe — a deployment is in progress (images already cleaned)")
	} else if bkOK, out := h.images.PruneBuildKitAll(ctx); bkOK {
		results = append(results, "✓ BuildKit cache cleared (builder prune -af)")
	} else {
		if out == "" {
			out = "failed"
		}
		results = append(results, "! BuildKit prune warning: "+out)
	}
	results = append(results, "○ Foreign images/containers/volumes were not touched",
		"○ Host OS maintenance (swap, journal, apt, /tmp) is not available via the panel")

	h.stats.Invalidate()
	memoryAfter, _ := h.stats.MemoryPercent(ctx)

	body := purgeBody{Message: "DockLift-scoped cleanup completed", Details: results,
		RemovedImages: len(res.Removed), KeptImages: len(res.Kept)}
	if memoryBefore != 0 && memoryBefore == memoryBefore {
		body.MemoryBefore = &memoryBefore
	}
	if memoryAfter != 0 && memoryAfter == memoryAfter {
		body.MemoryAfter = &memoryAfter
	}
	if body.MemoryBefore != nil && body.MemoryAfter != nil {
		saved := fmt.Sprintf("%g%%", memoryBefore-memoryAfter)
		body.MemorySaved = &saved
	}
	httpx.JSON(w, http.StatusOK, body)
}

// addContainerImages adds the image reference and image id of every
// container on the host to inUse.
func (h *handlers) addContainerImages(ctx context.Context, inUse map[string]bool) error {
	if h.docker == nil {
		return logs.ErrDockerNotConfigured
	}
	list, err := h.docker.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	for _, c := range list.Items {
		for _, ref := range []string{c.Image, c.ImageID} {
			if ref != "" {
				inUse[ref] = true
			}
		}
	}
	return nil
}

// purgeFailed answers like the route's catch block, with 503 when Docker is
// unreachable (deliberate fix 9).
func (h *handlers) purgeFailed(w http.ResponseWriter, err error) {
	if logs.IsDockerUnavailable(err) {
		h.d.Log.Warn("Purge error: Docker unavailable", "error", err)
		httpx.Error(w, http.StatusServiceUnavailable, logs.DockerUnavailableMessage)
		return
	}
	h.d.Log.Error("Purge error", "error", err)
	httpx.Error(w, http.StatusInternalServerError, "Failed to complete purge operation")
}
