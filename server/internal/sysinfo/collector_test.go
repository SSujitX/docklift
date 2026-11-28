package sysinfo_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"docklift/internal/httpx"
	"docklift/internal/sysinfo"
	"docklift/internal/sysinfo/sysinfotest"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func linuxSource() *sysinfotest.Source {
	rx := 1500.5
	return &sysinfotest.Source{
		CPUInfo:    sysinfo.CPUInfo{Brand: "container cpu", Cores: 2, Speed: 1.1},
		Load:       12.345,
		MemInfo:    sysinfo.MemInfo{Total: 8e9, Free: 2e9, Used: 6e9, SwapTotal: 1e9, SwapUsed: 2.5e8},
		Disks:      []sysinfo.FsEntry{{FS: "overlay", Type: "overlay", Size: 2e11, Used: 5e10, Use: 25.25, Mount: "/"}, {FS: "tmp", Type: "ext4", Size: 1e6, Mount: "/small"}},
		Nets:       []sysinfo.NetStat{{RxBytes: 1000, TxBytes: 2000, RxSec: &rx}},
		OS:         sysinfo.OSInfo{Hostname: "container-id", Platform: "linux", Distro: "Alpine Linux", Kernel: "6.8.0-45-generic", Arch: "x64"},
		UptimeSecs: 2*86400 + 3*3600 + 5*60 + 0.5,
		Loads:      [3]float64{0.555, 1.004, 2},
		IPv4:       "172.18.0.5",
		HostFiles: map[string]string{
			sysinfo.HostHostnamePath:  "my-vps",
			sysinfo.HostOSReleasePath: "PRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\nVERSION_ID=\"24.04\"",
			sysinfo.HostCPUInfoPath:   "processor : 0\nmodel name : AMD EPYC 7B13\ncpu MHz : 2450.000\nprocessor : 1\nprocessor : 2\nprocessor : 3",
			sysinfo.HostMemInfoPath:   "MemTotal: 16000000 kB\nMemFree: 1000000 kB\nMemAvailable: 12000000 kB",
		},
		HostProcs:   []sysinfo.Process{{PID: 1, Name: "/sbin/init", CPU: 1.5, Mem: 0.2, User: "root"}},
		Connections: 7,
	}
}

func toMap(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := httpx.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestStatsLinuxUsesHostFiles(t *testing.T) {
	src := linuxSource()
	clk := &clock{t: time.Date(2026, 11, 27, 15, 4, 5, 0, time.UTC)}
	c := sysinfo.NewCollector(t.Context(), src, quiet, "linux", sysinfo.Options{Now: clk.now, PublicIPURL: "http://127.0.0.1:1/"})
	s, err := c.Stats(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	m := toMap(t, s)
	cpu := m["cpu"].(map[string]any)
	if cpu["usage"] != 12.3 || cpu["cores"] != 4.0 || cpu["model"] != "AMD EPYC 7B13" || cpu["speed"] != 2.5 || cpu["temperature"] != nil {
		t.Fatalf("cpu: %v", cpu)
	}
	memory := m["memory"].(map[string]any)
	// Host meminfo: used = total - available.
	if memory["total"] != 16000000.0*1024 || memory["used"] != 4000000.0*1024 || memory["free"] != 12000000.0*1024 || memory["usedPercent"] != 25.0 {
		t.Fatalf("memory: %v", memory)
	}
	disks := m["disk"].([]any)
	if len(disks) != 1 || disks[0].(map[string]any)["usedPercent"] != 25.3 || disks[0].(map[string]any)["mount"] != "/" {
		t.Fatalf("disk: %v", disks)
	}
	if net := m["network"].(map[string]any); net["rxSpeed"] != 1500.5 || net["txSpeed"] != 0.0 || net["bytesSent"] != 2000.0 {
		t.Fatalf("network: %v", net)
	}
	srv := m["server"].(map[string]any)
	want := map[string]any{
		"hostname": "my-vps", "platform": "linux", "distro": "Ubuntu 24.04.1 LTS", "kernel": "6.8.0-45-generic", "arch": "x64",
		"uptimeFormatted": "2 Days, 3 Hours, 5 Mins", "serverTime": "11/27/2026, 3:04:05 PM", "cpuModel": "AMD EPYC 7B13",
		"cpuCores": "4 Cores @ 2.5 GHz", "ipAddress": "172.18.0.5", "location": "N/A",
	}
	for k, v := range want {
		if srv[k] != v {
			t.Errorf("server.%s = %v, want %v", k, srv[k], v)
		}
	}
	if la := srv["loadAvg"].(map[string]any); la["load1"] != 0.56 || la["load5"] != 1.0 || la["load15"] != 2.0 {
		t.Fatalf("loadAvg: %v", la)
	}
	if sw := srv["swap"].(map[string]any); sw["total"] != 1e9 || sw["used"] != 2.5e8 {
		t.Fatalf("swap: %v", sw)
	}
	if gpu := m["gpu"].(map[string]any); gpu["available"] != false || gpu["model"] != nil {
		t.Fatalf("gpu: %v", gpu)
	}
	if procs := m["processes"].([]any); len(procs) != 1 || procs[0].(map[string]any)["name"] != "/sbin/init" {
		t.Fatalf("processes: %v", procs)
	}
	if m["timestamp"] != "2026-11-27T15:04:05.000Z" {
		t.Fatalf("timestamp %v", m["timestamp"])
	}
	if src.CallCount("Processes") != 0 {
		t.Fatal("host processes were available; the local list must not be read")
	}
}

func TestStatsWindowsFallbacks(t *testing.T) {
	src := linuxSource()
	src.HostFiles = nil
	src.HostProcs = []sysinfo.Process{{PID: 9}}
	temp := 50.0
	src.Temp = &temp
	src.GPUInfo = &sysinfo.GPUController{Model: "gpu"}
	src.Procs = []sysinfo.RawProcess{
		{PID: 0, Name: "System Idle Process", CPU: 99},
		{PID: 1, Name: "a", CPU: 1.04, Mem: 2.25, User: "u"},
		{PID: 2, Name: "b", CPU: 7.56, Mem: 0},
	}
	for i := 0; i < 12; i++ {
		src.Procs = append(src.Procs, sysinfo.RawProcess{PID: 100 + i, Name: "idle"})
		src.Procs = append(src.Procs, sysinfo.RawProcess{PID: 200 + i, Name: "x"})
	}
	c := sysinfo.NewCollector(t.Context(), src, quiet, "windows", sysinfo.Options{PublicIPURL: "http://127.0.0.1:1/"})
	s, err := c.Stats(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if s.CPU.Temperature != nil || s.GPU.Available || src.CallCount("CPUTemperature") != 0 || src.CallCount("GPU") != 0 {
		t.Fatal("temperature and GPU are skipped on Windows")
	}
	if src.CallCount("HostProcesses") != 0 || src.CallCount("EstablishedConnections") != 0 {
		t.Fatal("nsenter processes and connection counting are skipped on Windows")
	}
	if len(s.Processes) != 10 || s.Processes[0].Name != "b" || s.Processes[0].CPU != 7.6 || s.Processes[1].Mem != 2.3 {
		t.Fatalf("processes: %+v", s.Processes)
	}
	if s.Memory.Used != 6e9 || s.Memory.UsedPercent != 75 || s.CPU.Model != "container cpu" || s.Server.Hostname != "container-id" || s.Server.Distro != "Alpine Linux" {
		t.Fatalf("container fallbacks: %+v", s)
	}
}

func TestStatsCacheAndInvalidate(t *testing.T) {
	src := linuxSource()
	clk := &clock{t: time.Now()}
	c := sysinfo.NewCollector(t.Context(), src, quiet, "linux", sysinfo.Options{Now: clk.now, PublicIPURL: "http://127.0.0.1:1/"})
	first, _ := c.Stats(t.Context())
	clk.add(2 * time.Second)
	second, _ := c.Stats(t.Context())
	if first != second || src.CallCount("CPU") != 1 {
		t.Fatal("stats younger than 3 s must come from the cache")
	}
	clk.add(2 * time.Second)
	if third, _ := c.Stats(t.Context()); third == first || src.CallCount("CPU") != 2 {
		t.Fatal("stale stats must be recomputed")
	}
	c.Invalidate()
	_, _ = c.Stats(t.Context())
	if src.CallCount("CPU") != 3 {
		t.Fatal("Invalidate must drop the cache")
	}
}

func TestStatsErrors(t *testing.T) {
	for name, mutate := range map[string]func(*sysinfotest.Source){
		"cpu":  func(s *sysinfotest.Source) { s.CPUErr = errors.New("boom") },
		"load": func(s *sysinfotest.Source) { s.LoadErr = errors.New("boom") },
		"mem":  func(s *sysinfotest.Source) { s.MemErr = errors.New("boom") },
	} {
		src := linuxSource()
		mutate(src)
		c := sysinfo.NewCollector(t.Context(), src, quiet, "linux", sysinfo.Options{PublicIPURL: "http://127.0.0.1:1/"})
		if _, err := c.Stats(t.Context()); err == nil {
			t.Errorf("%s error must fail the stats", name)
		}
	}
}

func TestActiveConnectionsRefreshInBackground(t *testing.T) {
	src := linuxSource()
	c := sysinfo.NewCollector(t.Context(), src, quiet, "linux", sysinfo.Options{PublicIPURL: "http://127.0.0.1:1/"})
	deadline := time.Now().Add(5 * time.Second)
	for {
		c.Invalidate()
		s, err := c.Stats(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if s.Server.ActiveConnections == 7 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("connection count never arrived")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := src.CallCount("EstablishedConnections"); n != 1 {
		t.Fatalf("counted %d times; the 30 s cache must hold", n)
	}
}

func TestPublicIP(t *testing.T) {
	var hits int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		_, _ = io.WriteString(w, `{"ip":"203.0.113.9"}`)
	}))
	defer srv.Close()
	src := linuxSource()
	c := sysinfo.NewCollector(t.Context(), src, quiet, "linux", sysinfo.Options{PublicIPURL: srv.URL})
	if ip := c.PublicIP(t.Context()); ip != "203.0.113.9" {
		t.Fatalf("ip = %q", ip)
	}
	if ip := c.PublicIP(t.Context()); ip != "203.0.113.9" {
		t.Fatalf("cached ip = %q", ip)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Fatalf("lookups = %d, want 1 (5 minute cache)", hits)
	}
	s, _ := c.Stats(t.Context())
	if s.Server.IPAddress != "203.0.113.9" {
		t.Fatalf("stats ip %q", s.Server.IPAddress)
	}
}

func TestPublicIPFailureKeepsNA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer srv.Close()
	c := sysinfo.NewCollector(t.Context(), linuxSource(), quiet, "linux", sysinfo.Options{PublicIPURL: srv.URL})
	if ip := c.PublicIP(t.Context()); ip != "N/A" {
		t.Fatalf("ip = %q", ip)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if ip := c.PublicIP(ctx); ip != "N/A" {
		t.Fatalf("cancelled lookup ip = %q", ip)
	}
}

func TestQuickAndMemoryPercent(t *testing.T) {
	src := linuxSource()
	clk := &clock{t: time.Date(2026, 1, 2, 3, 4, 5, 678_000_000, time.UTC)}
	c := sysinfo.NewCollector(t.Context(), src, quiet, "linux", sysinfo.Options{Now: clk.now})
	q, err := c.Quick(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// Memory is total - available of the host meminfo (the figure Stats
	// reports), not total - free: page cache is not used memory.
	if got := toMap(t, q); got["cpu"] != 12.3 || got["memory"] != 25.0 || got["timestamp"] != "2026-01-02T03:04:05.678Z" {
		t.Fatalf("quick: %v", got)
	}
	src.MemInfo.Used = 4.5e9 / 1.0 // 56.25 %
	if p, _ := c.MemoryPercent(t.Context()); p != 56 {
		t.Fatalf("memory percent %v", p)
	}
	src.MemErr = errors.New("x")
	if _, err := c.Quick(t.Context()); err == nil {
		t.Fatal("quick must fail with memory")
	}
}
