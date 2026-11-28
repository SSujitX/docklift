package system

import (
	"context"

	"docklift/internal/deploy"
)

// runtimeImages is the production ImageCleanup: the keep-2 image retention
// of the process-wide deploy runtime (Deps.Deploy).
type runtimeImages struct{ rt *deploy.Runtime }

var _ ImageCleanup = runtimeImages{}

func (i runtimeImages) KeepTags(ctx context.Context, projectID string) ([]string, error) {
	set, err := i.rt.ProjectKeepSet(ctx, projectID, nil)
	if err != nil {
		return nil, err
	}
	tags := make([]string, 0, len(set))
	for t := range set {
		tags = append(tags, t)
	}
	return tags, nil
}

func (i runtimeImages) ContainerImageRef(ctx context.Context, containerName string) string {
	return i.rt.Docker.ContainerImageRef(ctx, containerName)
}

func (i runtimeImages) RemoveUnusedImages(ctx context.Context, keep, inUse map[string]bool) (CleanupResult, error) {
	res, err := i.rt.RemoveUnusedDockliftImages(ctx, "", keep, inUse)
	return CleanupResult{Kept: res.Kept, Removed: res.Removed, SkippedInUse: res.SkippedInUse, Errors: res.Errors}, err
}

func (i runtimeImages) PruneBuildKitAll(ctx context.Context) (bool, string) {
	res := i.rt.Docker.BuilderPrune(ctx, true)
	return res.OK, res.Output
}
