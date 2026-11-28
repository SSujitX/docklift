//go:build !linux

package sysinfo

import (
	"net"
	"testing"
)

func TestInterfaceWithIP(t *testing.T) {
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Skip(err)
	}
	if got := interfaceWithIP(ifaces, net.IPv4(127, 0, 0, 1)); got == "" {
		t.Skip("no loopback interface with 127.0.0.1 on this machine")
	}
	if got := interfaceWithIP(ifaces, net.ParseIP("192.0.2.200")); got != "" {
		t.Fatalf("TEST-NET address matched interface %q", got)
	}
}

func TestDefaultInterfaceExists(t *testing.T) {
	name := defaultInterface()
	if name == "" {
		t.Skip("no usable network interface")
	}
	if _, err := net.InterfaceByName(name); err != nil {
		t.Fatalf("defaultInterface() = %q, not an interface: %v", name, err)
	}
}
