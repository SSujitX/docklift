// Package version carries the release version reported by /api/health.
package version

// Version is overridden at build time with
//
//	go build -ldflags "-X docklift/internal/version.Version=<semver>"
//
// and defaults to the version of backend/package.json this port matches.
var Version = "2.1.6"
