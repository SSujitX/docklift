// Package deps pins modules that the port depends on but that no package
// imports yet, so `go mod tidy` keeps them in go.mod while route groups are
// ported in parallel. Delete an import here as soon as a real package imports
// that module; delete this package when it is empty.
package deps

import (
	_ "github.com/creack/pty"
	_ "github.com/gorilla/websocket"
	_ "github.com/moby/moby/api/types/container"
	_ "github.com/shirou/gopsutil/v4/cpu"
	_ "github.com/shirou/gopsutil/v4/disk"
	_ "github.com/shirou/gopsutil/v4/host"
	_ "github.com/shirou/gopsutil/v4/mem"
	_ "github.com/shirou/gopsutil/v4/net"
	_ "gopkg.in/yaml.v3"
)
