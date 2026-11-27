// Package sysinfotest provides a scripted sysinfo.Source for tests.
package sysinfotest

import (
	"context"
	"sync"

	"docklift/internal/sysinfo"
)

// Source is a sysinfo.Source returning its fields. Calls are counted per
// method so tests can check caching.
type Source struct {
	mu sync.Mutex

	CPUInfo     sysinfo.CPUInfo
	CPUErr      error
	Load        float64
	LoadErr     error
	Temp        *float64
	MemInfo     sysinfo.MemInfo
	MemErr      error
	Disks       []sysinfo.FsEntry
	Nets        []sysinfo.NetStat
	OS          sysinfo.OSInfo
	GPUInfo     *sysinfo.GPUController
	UptimeSecs  float64
	Loads       [3]float64
	IPv4        string
	HostFiles   map[string]string
	HostProcs   []sysinfo.Process
	Procs       []sysinfo.RawProcess
	Connections int
	ConnErr     error

	Calls map[string]int
}

func (s *Source) count(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Calls == nil {
		s.Calls = map[string]int{}
	}
	s.Calls[name]++
}

// CallCount returns how often a method ran.
func (s *Source) CallCount(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Calls[name]
}

// CPU implements sysinfo.Source.
func (s *Source) CPU(context.Context) (sysinfo.CPUInfo, error) {
	s.count("CPU")
	return s.CPUInfo, s.CPUErr
}

// CurrentLoad implements sysinfo.Source.
func (s *Source) CurrentLoad(context.Context) (float64, error) {
	s.count("CurrentLoad")
	return s.Load, s.LoadErr
}

// CPUTemperature implements sysinfo.Source.
func (s *Source) CPUTemperature(context.Context) *float64 {
	s.count("CPUTemperature")
	return s.Temp
}

// Mem implements sysinfo.Source.
func (s *Source) Mem(context.Context) (sysinfo.MemInfo, error) {
	s.count("Mem")
	return s.MemInfo, s.MemErr
}

// FsSize implements sysinfo.Source.
func (s *Source) FsSize(context.Context) []sysinfo.FsEntry { return s.Disks }

// NetworkStats implements sysinfo.Source.
func (s *Source) NetworkStats(context.Context) []sysinfo.NetStat { return s.Nets }

// OSInfo implements sysinfo.Source.
func (s *Source) OSInfo(context.Context) sysinfo.OSInfo { return s.OS }

// GPU implements sysinfo.Source.
func (s *Source) GPU(context.Context) *sysinfo.GPUController {
	s.count("GPU")
	return s.GPUInfo
}

// Uptime implements sysinfo.Source.
func (s *Source) Uptime(context.Context) float64 { return s.UptimeSecs }

// LoadAvg implements sysinfo.Source.
func (s *Source) LoadAvg(context.Context) [3]float64 { return s.Loads }

// FirstIPv4 implements sysinfo.Source.
func (s *Source) FirstIPv4(context.Context) string { return s.IPv4 }

// ReadHostFile implements sysinfo.Source.
func (s *Source) ReadHostFile(path string) (string, bool) {
	v, ok := s.HostFiles[path]
	return v, ok
}

// HostProcesses implements sysinfo.Source.
func (s *Source) HostProcesses(context.Context) []sysinfo.Process {
	s.count("HostProcesses")
	return s.HostProcs
}

// Processes implements sysinfo.Source.
func (s *Source) Processes(context.Context) []sysinfo.RawProcess {
	s.count("Processes")
	return s.Procs
}

// EstablishedConnections implements sysinfo.Source.
func (s *Source) EstablishedConnections(context.Context) (int, error) {
	s.count("EstablishedConnections")
	return s.Connections, s.ConnErr
}
