// Package sysinfo collects the host statistics served by /api/system/stats
// and /api/system/quick (the getSystemStats logic of
// backend/src/routes/system.ts). Source is the systeminformation-equivalent
// layer (gopsutil, /proc, df, nsenter); Collector combines its readings into
// the exact JSON shape, rounding and caching of the TypeScript backend.
package sysinfo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Host files the TypeScript backend read from the bind-mounted host root.
const (
	HostHostnamePath  = "/host/hostname"
	HostOSReleasePath = "/host/os-release"
	HostCPUInfoPath   = "/host/proc/cpuinfo"
	HostMemInfoPath   = "/host/proc/meminfo"
)

// PublicIPURL is the HTTPS IP discovery API.
const PublicIPURL = "https://api.ipify.org?format=json"

const (
	statsCacheTTL       = 3 * time.Second
	connectionsCacheTTL = 30 * time.Second
	publicIPCacheTTL    = 5 * time.Minute
	publicIPTimeout     = 10 * time.Second
	minDiskBytes        = 1024 * 1024 * 100
)

// CPUInfo is si.cpu(): brand (vendor removed), logical cores, speed in GHz.
type CPUInfo struct {
	Brand string
	Cores int
	Speed float64
}

// MemInfo is si.mem() in bytes; Used is Total - Free (buffers and cache
// count as used, as in systeminformation).
type MemInfo struct {
	Total     float64
	Free      float64
	Used      float64
	SwapTotal float64
	SwapUsed  float64
}

// NetStat is si.networkStats() for the default interface; RxSec/TxSec are
// bytes per second since the previous reading, nil on the first one.
type NetStat struct {
	RxBytes float64
	TxBytes float64
	RxSec   *float64
	TxSec   *float64
}

// OSInfo is si.osInfo().
type OSInfo struct {
	Hostname string
	Platform string
	Distro   string
	Kernel   string
	Arch     string
}

// GPUController is the first entry of si.graphics().controllers.
type GPUController struct {
	Model          string
	VRAM           float64
	MemoryUsed     float64
	TemperatureGPU float64
	UtilizationGPU float64
}

// RawProcess is one entry of si.processes().list.
type RawProcess struct {
	PID  int
	Name string
	CPU  float64
	Mem  float64
	User string
}

// Source reads raw host facts. Implementations must be safe for concurrent
// use; readings that are unavailable come back as zero values.
type Source interface {
	CPU(ctx context.Context) (CPUInfo, error)
	CurrentLoad(ctx context.Context) (float64, error)
	CPUTemperature(ctx context.Context) *float64
	Mem(ctx context.Context) (MemInfo, error)
	FsSize(ctx context.Context) []FsEntry
	NetworkStats(ctx context.Context) []NetStat
	OSInfo(ctx context.Context) OSInfo
	GPU(ctx context.Context) *GPUController
	Uptime(ctx context.Context) float64
	LoadAvg(ctx context.Context) [3]float64
	FirstIPv4(ctx context.Context) string
	// ReadHostFile returns the trimmed content of a host file, false when it
	// cannot be read.
	ReadHostFile(path string) (string, bool)
	// HostProcesses is the Linux process table of the host (nsenter ps,
	// falling back to top); nil when unavailable.
	HostProcesses(ctx context.Context) []Process
	Processes(ctx context.Context) []RawProcess
	EstablishedConnections(ctx context.Context) (int, error)
}

// Stats is the /api/system/stats response.
type Stats struct {
	CPU       CPUStats  `json:"cpu"`
	Memory    MemStats  `json:"memory"`
	GPU       GPUStats  `json:"gpu"`
	Disk      []Disk    `json:"disk"`
	Network   Network   `json:"network"`
	Server    Server    `json:"server"`
	Processes []Process `json:"processes"`
	Timestamp string    `json:"timestamp"`
}

// CPUStats is Stats.cpu.
type CPUStats struct {
	Usage       Num      `json:"usage"`
	Cores       int      `json:"cores"`
	Model       string   `json:"model"`
	Speed       Num      `json:"speed"`
	Temperature *float64 `json:"temperature"`
}

// MemStats is Stats.memory.
type MemStats struct {
	Total       Num `json:"total"`
	Used        Num `json:"used"`
	Free        Num `json:"free"`
	UsedPercent Num `json:"usedPercent"`
}

// GPUStats is Stats.gpu.
type GPUStats struct {
	Available   bool     `json:"available"`
	Model       *string  `json:"model"`
	MemoryTotal *float64 `json:"memoryTotal"`
	MemoryUsed  *float64 `json:"memoryUsed"`
	Temperature *float64 `json:"temperature"`
	Utilization *float64 `json:"utilization"`
}

// Disk is one Stats.disk entry.
type Disk struct {
	Mount       string `json:"mount"`
	Type        string `json:"type"`
	Total       Num    `json:"total"`
	Used        Num    `json:"used"`
	UsedPercent Num    `json:"usedPercent"`
}

// Network is Stats.network.
type Network struct {
	BytesReceived Num `json:"bytesReceived"`
	BytesSent     Num `json:"bytesSent"`
	RxSpeed       Num `json:"rxSpeed"`
	TxSpeed       Num `json:"txSpeed"`
}

// Server is Stats.server.
type Server struct {
	Hostname          string  `json:"hostname"`
	Platform          string  `json:"platform"`
	Distro            string  `json:"distro"`
	Kernel            string  `json:"kernel"`
	Arch              string  `json:"arch"`
	Uptime            Num     `json:"uptime"`
	UptimeFormatted   string  `json:"uptimeFormatted"`
	ServerTime        string  `json:"serverTime"`
	CPUModel          string  `json:"cpuModel"`
	CPUCores          string  `json:"cpuCores"`
	LoadAvg           LoadAvg `json:"loadAvg"`
	Swap              Swap    `json:"swap"`
	IPAddress         string  `json:"ipAddress"`
	Location          string  `json:"location"`
	ActiveConnections int     `json:"activeConnections"`
}

// LoadAvg is Stats.server.loadAvg.
type LoadAvg struct {
	Load1  Num `json:"load1"`
	Load5  Num `json:"load5"`
	Load15 Num `json:"load15"`
}

// Swap is Stats.server.swap.
type Swap struct {
	Total Num `json:"total"`
	Used  Num `json:"used"`
}

// Quick is the /api/system/quick response.
type Quick struct {
	CPU       Num    `json:"cpu"`
	Memory    Num    `json:"memory"`
	Timestamp string `json:"timestamp"`
}

// Collector caches host statistics like the module-level caches of
// system.ts: stats for 3 s, the established-connection count for 30 s
// (refreshed in the background) and the public IP for 5 minutes.
type Collector struct {
	src        Source
	log        *slog.Logger
	background context.Context
	goos       string
	now        func() time.Time
	ipURL      string
	http       *http.Client

	statsMu   sync.Mutex
	cached    *Stats
	lastFetch time.Time

	connMu       sync.Mutex
	conns        int
	lastConnsAt  time.Time
	connsRunning bool

	ipMu       sync.Mutex
	publicIP   string
	location   string
	lastIPAt   time.Time
	ipInFlight chan struct{}
}

// Options configure a Collector; zero fields take production defaults.
type Options struct {
	// GOOS replaces runtime.GOOS (platform branches of system.ts).
	GOOS string
	// Now replaces time.Now.
	Now func() time.Time
	// PublicIPURL replaces PublicIPURL.
	PublicIPURL string
	// HTTPClient performs the public IP lookup.
	HTTPClient *http.Client
}

// NewCollector returns a collector reading from src. Background refreshes
// stop when background is cancelled.
func NewCollector(background context.Context, src Source, log *slog.Logger, goos string, opts Options) *Collector {
	c := &Collector{src: src, log: log, background: background, goos: goos, now: time.Now,
		ipURL: PublicIPURL, http: &http.Client{Timeout: publicIPTimeout}, publicIP: "N/A", location: "N/A"}
	if opts.GOOS != "" {
		c.goos = opts.GOOS
	}
	if opts.Now != nil {
		c.now = opts.Now
	}
	if opts.PublicIPURL != "" {
		c.ipURL = opts.PublicIPURL
	}
	if opts.HTTPClient != nil {
		c.http = opts.HTTPClient
	}
	return c
}

// Invalidate drops the cached stats (after a purge).
func (c *Collector) Invalidate() {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	c.cached = nil
	c.lastFetch = time.Time{}
}

func orNull(v float64) *float64 {
	if v == 0 || math.IsNaN(v) {
		return nil
	}
	return &v
}

// Stats returns the cached statistics when younger than 3 seconds, else a
// fresh reading.
func (c *Collector) Stats(ctx context.Context) (*Stats, error) {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	now := c.now()
	if c.cached != nil && now.Sub(c.lastFetch) < statsCacheTTL {
		return c.cached, nil
	}
	windows := c.goos == "windows"
	if !windows {
		c.refreshConnections(now)
	}
	if c.ipStale(now) {
		c.refreshPublicIPAsync()
	}

	cpuData, err := c.src.CPU(ctx)
	if err != nil {
		return nil, fmt.Errorf("cpu info: %w", err)
	}
	load, err := c.src.CurrentLoad(ctx)
	if err != nil {
		return nil, fmt.Errorf("cpu load: %w", err)
	}
	memData, err := c.src.Mem(ctx)
	if err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}
	var cpuTemp *float64
	var gpu *GPUController
	if !windows {
		if t := c.src.CPUTemperature(ctx); t != nil {
			cpuTemp = orNull(*t)
		}
		gpu = c.src.GPU(ctx)
	}
	osInfo := c.src.OSInfo(ctx)
	uptime := c.src.Uptime(ctx)

	hostname, _ := c.src.ReadHostFile(HostHostnamePath)
	var hostOS *HostOS
	if content, ok := c.src.ReadHostFile(HostOSReleasePath); ok && content != "" {
		parsed := ParseOSRelease(content)
		hostOS = &parsed
	}
	var hostCPU *HostCPU
	if content, ok := c.src.ReadHostFile(HostCPUInfoPath); ok && content != "" {
		parsed := ParseCPUInfo(content)
		hostCPU = &parsed
	}
	var hostMem *HostMem
	if content, ok := c.src.ReadHostFile(HostMemInfoPath); ok && content != "" {
		parsed := ParseMemInfo(content)
		hostMem = &parsed
	}
	var linuxProcs []Process
	if c.goos == "linux" {
		linuxProcs = c.src.HostProcesses(ctx)
	}

	s := &Stats{Disk: []Disk{}}
	s.GPU = GPUStats{Available: gpu != nil}
	if gpu != nil {
		if gpu.Model != "" {
			model := gpu.Model
			s.GPU.Model = &model
		}
		s.GPU.MemoryTotal = orNull(gpu.VRAM)
		s.GPU.MemoryUsed = orNull(gpu.MemoryUsed)
		s.GPU.Temperature = orNull(gpu.TemperatureGPU)
		s.GPU.Utilization = orNull(gpu.UtilizationGPU)
	}
	for _, d := range c.src.FsSize(ctx) {
		if d.Size > minDiskBytes {
			s.Disk = append(s.Disk, Disk{Mount: d.Mount, Type: d.Type, Total: Num(d.Size), Used: Num(d.Used), UsedPercent: Num(ToFixed(d.Use, 1))})
		}
	}
	if nets := c.src.NetworkStats(ctx); len(nets) > 0 {
		n := nets[0]
		s.Network = Network{BytesReceived: Num(n.RxBytes), BytesSent: Num(n.TxBytes), RxSpeed: Num(orZero(n.RxSec)), TxSpeed: Num(orZero(n.TxSec))}
	}
	if len(linuxProcs) > 0 {
		s.Processes = linuxProcs
	} else {
		s.Processes = topProcesses(c.src.Processes(ctx))
	}
	if len(s.Processes) > 10 {
		s.Processes = s.Processes[:10]
	}

	memUsed, memTotal, memFree := memData.Used, memData.Total, memData.Free
	if hostMem != nil {
		memUsed = hostMem.Total - hostMem.Available
		if hostMem.Total != 0 {
			memTotal = hostMem.Total
		}
		if hostMem.Available != 0 {
			memFree = hostMem.Available
		}
	}
	cores, speed, model := cpuData.Cores, cpuData.Speed, cpuData.Brand
	if hostCPU != nil {
		if hostCPU.Cores != 0 {
			cores = hostCPU.Cores
		}
		if hostCPU.Speed != 0 {
			speed = hostCPU.Speed
		}
		if hostCPU.Model != "" {
			model = hostCPU.Model
		}
	}
	s.CPU = CPUStats{Usage: Num(ToFixed(load, 1)), Cores: cores, Model: model, Speed: Num(speed), Temperature: cpuTemp}
	s.Memory = MemStats{Total: Num(memTotal), Used: Num(memUsed), Free: Num(memFree), UsedPercent: Num(ToFixed(memUsed/memTotal*100, 1))}

	loads := c.src.LoadAvg(ctx)
	c.ipMu.Lock()
	publicIP, location := c.publicIP, c.location
	c.ipMu.Unlock()
	ip := publicIP
	if ip == "N/A" {
		if first := c.src.FirstIPv4(ctx); first != "" {
			ip = first
		}
	}
	c.connMu.Lock()
	conns := c.conns
	c.connMu.Unlock()
	s.Server = Server{
		Hostname:          osInfo.Hostname,
		Platform:          osInfo.Platform,
		Distro:            osInfo.Distro,
		Kernel:            osInfo.Kernel,
		Arch:              osInfo.Arch,
		Uptime:            Num(uptime),
		UptimeFormatted:   FormatUptime(uptime),
		ServerTime:        LocaleString(now),
		CPUModel:          model,
		CPUCores:          fmt.Sprintf("%d Cores @ %s GHz", cores, FormatNumber(speed)),
		LoadAvg:           LoadAvg{Load1: Num(ToFixed(loads[0], 2)), Load5: Num(ToFixed(loads[1], 2)), Load15: Num(ToFixed(loads[2], 2))},
		Swap:              Swap{Total: Num(memData.SwapTotal), Used: Num(memData.SwapUsed)},
		IPAddress:         ip,
		Location:          location,
		ActiveConnections: conns,
	}
	if hostname != "" {
		s.Server.Hostname = hostname
	}
	if hostOS != nil {
		s.Server.Platform, s.Server.Distro = hostOS.Platform, hostOS.Distro
	}
	s.Timestamp = ISOString(c.now())

	c.cached = s
	c.lastFetch = now
	return s, nil
}

func orZero(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// topProcesses is the si.processes() fallback: idle pseudo-processes
// removed, the ten busiest first, percentages rounded to one decimal.
func topProcesses(list []RawProcess) []Process {
	filtered := make([]RawProcess, 0, len(list))
	for _, p := range list {
		if p.Name != "System Idle Process" && p.Name != "idle" {
			filtered = append(filtered, p)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].CPU > filtered[j].CPU })
	if len(filtered) > 10 {
		filtered = filtered[:10]
	}
	out := make([]Process, 0, len(filtered))
	for _, p := range filtered {
		out = append(out, Process{PID: p.PID, Name: p.Name, CPU: Num(ToFixed(p.CPU, 1)), Mem: Num(ToFixed(p.Mem, 1)), User: p.User})
	}
	return out
}

// Quick is the header widget reading: CPU load and memory use in percent.
func (c *Collector) Quick(ctx context.Context) (*Quick, error) {
	load, err := c.src.CurrentLoad(ctx)
	if err != nil {
		return nil, fmt.Errorf("cpu load: %w", err)
	}
	m, err := c.src.Mem(ctx)
	if err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}
	return &Quick{CPU: Num(ToFixed(load, 1)), Memory: Num(ToFixed(m.Used/m.Total*100, 1)), Timestamp: ISOString(c.now())}, nil
}

// MemoryPercent is Math.round(used / total * 100) of si.mem() (purge
// before/after figures).
func (c *Collector) MemoryPercent(ctx context.Context) (float64, error) {
	m, err := c.src.Mem(ctx)
	if err != nil {
		return 0, err
	}
	return RoundHalfUp(m.Used / m.Total * 100), nil
}

// refreshConnections starts a background count of established connections
// when the cached one is older than 30 seconds.
func (c *Collector) refreshConnections(now time.Time) {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.connsRunning || now.Sub(c.lastConnsAt) <= connectionsCacheTTL {
		return
	}
	c.connsRunning = true
	go func() {
		n, err := c.src.EstablishedConnections(c.background)
		c.connMu.Lock()
		defer c.connMu.Unlock()
		c.connsRunning = false
		if err != nil {
			return
		}
		c.conns = n
		c.lastConnsAt = c.now()
	}()
}

func (c *Collector) ipStale(now time.Time) bool {
	c.ipMu.Lock()
	defer c.ipMu.Unlock()
	return now.Sub(c.lastIPAt) > publicIPCacheTTL
}

// PublicIP is GET /api/system/ip: the cached public IP, looked up first when
// unknown or older than five minutes ("N/A" when the lookup fails).
func (c *Collector) PublicIP(ctx context.Context) string {
	c.ipMu.Lock()
	need := c.publicIP == "N/A" || c.now().Sub(c.lastIPAt) > publicIPCacheTTL
	c.ipMu.Unlock()
	if need {
		select {
		case <-c.refreshPublicIPAsync():
		case <-ctx.Done():
		}
	}
	c.ipMu.Lock()
	defer c.ipMu.Unlock()
	return c.publicIP
}

// refreshPublicIPAsync starts (or joins) a lookup and returns a channel that
// is closed when it finishes.
func (c *Collector) refreshPublicIPAsync() <-chan struct{} {
	c.ipMu.Lock()
	defer c.ipMu.Unlock()
	if c.ipInFlight != nil {
		return c.ipInFlight
	}
	done := make(chan struct{})
	c.ipInFlight = done
	go func() {
		ip, err := c.fetchPublicIP(c.background)
		c.ipMu.Lock()
		defer c.ipMu.Unlock()
		if err != nil {
			c.log.Debug("public IP lookup failed", "error", err)
		} else if ip != nil {
			c.publicIP, c.location = *ip, "N/A"
		}
		c.lastIPAt = c.now()
		c.ipInFlight = nil
		close(done)
	}()
	return done
}

var errIPStatus = errors.New("unexpected status")

// fetchPublicIP returns the discovered address ("N/A" when the API answered
// without one). An error (network failure, non-2xx status, bad body) leaves
// the cached address unchanged, as fetchPublicIPInfo did.
func (c *Collector) fetchPublicIP(ctx context.Context) (*string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ipURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%w %d", errIPStatus, resp.StatusCode)
	}
	var body struct {
		IP any `json:"ip"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&body); err != nil {
		return nil, err
	}
	ip, _ := body.IP.(string)
	if ip == "" {
		ip = "N/A"
	}
	return &ip, nil
}
