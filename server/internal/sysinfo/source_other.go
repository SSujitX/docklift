//go:build !linux

package sysinfo

import (
	"context"
	"net"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	psnet "github.com/shirou/gopsutil/v4/net"
)

// hostProcesses is Linux-only (nsenter into PID 1).
func hostProcesses(context.Context) []Process { return nil }

// routeProbeAddr is a TEST-NET-1 address (RFC 5737): connecting a UDP socket
// to it makes the OS pick the source address of the default route without
// sending a packet.
const routeProbeAddr = "192.0.2.1:9"

// defaultInterface is the interface of the default route, like
// systeminformation's networkInterfaceDefault (which parses the routing
// table): the interface owning the source address the OS chooses for an
// outbound connection. Without a default route it is the first interface
// that is up, not a loopback and has an address.
func defaultInterface() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	if conn, err := net.Dial("udp", routeProbeAddr); err == nil {
		local, _ := conn.LocalAddr().(*net.UDPAddr)
		_ = conn.Close()
		if local != nil {
			if name := interfaceWithIP(ifaces, local.IP); name != "" {
				return name
			}
		}
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		if addrs, err := ifc.Addrs(); err == nil && len(addrs) > 0 {
			return ifc.Name
		}
	}
	return ""
}

// interfaceWithIP is the name of the interface that has ip, "" when none.
func interfaceWithIP(ifaces []net.Interface, ip net.IP) string {
	for _, ifc := range ifaces {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.Equal(ip) {
				return ifc.Name
			}
		}
	}
	return ""
}

func procUptime() (float64, bool) { return 0, false }

// fsSize lists mounted drives with their usage, like systeminformation's
// Windows (Win32_LogicalDisk) and macOS (df) readers.
func fsSize(ctx context.Context) []FsEntry {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return nil
	}
	data := []FsEntry{}
	for _, p := range parts {
		u, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		size, used, free := float64(u.Total), float64(u.Used), float64(u.Free)
		data = append(data, FsEntry{FS: p.Device, Type: p.Fstype, Size: size, Used: used, Available: free,
			Use: toFixed(100.0*used/(used+free), 2), Mount: p.Mountpoint})
	}
	return data
}

// gpu is not probed outside Linux (the TypeScript backend skipped it on
// Windows and ran in a Linux container in production).
func gpu(context.Context) *GPUController { return nil }

func establishedConnections(ctx context.Context) (int, error) {
	conns, err := psnet.ConnectionsWithContext(ctx, "inet")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, c := range conns {
		if c.Status == "ESTABLISHED" {
			n++
		}
	}
	return n, nil
}

func distro(ctx context.Context) string {
	info, err := host.InfoWithContext(ctx)
	if err != nil || info.Platform == "" {
		return "unknown"
	}
	return info.Platform
}
