// Package version carries the release version reported by /api/health.
package version

// Version is overridden at build time with
//
//	go build -ldflags "-X docklift/internal/version.Version=<semver>"
//
// The root VERSION file is the source of truth: the Dockerfile and
// `make build` read it and stamp it in. A plain `go build` reports the
// development default below.
var Version = "0.0.0-dev"
