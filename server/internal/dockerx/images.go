package dockerx

import (
	"context"
	"fmt"

	"github.com/moby/moby/client"
)

// Image is a local image with its repo tags ("<none>:<none>" removed).
type Image struct {
	ID   string
	Tags []string
}

// ListImages lists every local image (all: true, like dockerode
// listImages({all: true})).
func (d *Docker) ListImages(ctx context.Context) ([]Image, error) {
	if d.Engine == nil {
		return nil, ErrUnavailable
	}
	res, err := d.Engine.ImageList(ctx, client.ImageListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	out := make([]Image, 0, len(res.Items))
	for _, img := range res.Items {
		tags := []string{}
		for _, t := range img.RepoTags {
			if t != "" && t != "<none>:<none>" {
				tags = append(tags, t)
			}
		}
		out = append(out, Image{ID: img.ID, Tags: tags})
	}
	return out, nil
}

// ImageExists reports whether an image tag or id is present locally.
func (d *Docker) ImageExists(ctx context.Context, ref string) bool {
	if d.Engine == nil {
		return false
	}
	_, err := d.Engine.ImageInspect(ctx, ref)
	return err == nil
}

// RemoveImage force-removes an image by tag or id. It returns false without
// error when the image does not exist.
func (d *Docker) RemoveImage(ctx context.Context, ref string) (bool, error) {
	if d.Engine == nil {
		return false, ErrUnavailable
	}
	if _, err := d.Engine.ImageRemove(ctx, ref, client.ImageRemoveOptions{Force: true}); err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
