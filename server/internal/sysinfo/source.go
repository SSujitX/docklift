package sysinfo

import (
	"context"
	"fmt"
	"math"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	psnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/shirou/gopsutil/v4/sensors"
)

// HostSource is the production Source: gopsutil plus the OS-specific
// readers in source_linux.go / source_other.go.
type HostSource struct {
	netMu   sync.Mutex
	netLast map[string]netSample

	cpuMu   sync.Mutex
	cpuLast cpuSample
}

type netSample struct {
	rx, tx float64
	at     time.Time
}

// NewHostSource returns the production Source.
func NewHostSource() *HostSource {
	return &HostSource{netLast: map[string]netSample{}}
}

// CPU implements Source.
func (h *HostSource) CPU(ctx context.Context) (CPUInfo, error) {
	cores, err := cpu.CountsWithContext(ctx, true)
	if err != nil {
		return CPUInfo{}, fmt.Errorf("count cpus: %w", err)
	}
	info := CPUInfo{Cores: cores}
	if infos, err := cpu.InfoWithContext(ctx); err == nil && len(infos) > 0 {
		info.Brand = CPUBrand(infos[0].ModelName)
		info.Speed = CPUSpeedFromModel(infos[0].ModelName)
		if info.Speed == 0 && infos[0].Mhz > 0 {
			info.Speed = toFixed(infos[0].Mhz/1000, 2)
		}
	}
	return info, nil
}

// CurrentLoad implements Source: overall CPU busy percent since the previous
// call, and since boot on the first call, like si.currentLoad (gopsutil's
// cpu.Percent(0) would measure the first call from package init, which
// reports the server's own startup as ~100% busy).
func (h *HostSource) CurrentLoad(ctx context.Context) (float64, error) {
	times, err := cpu.TimesWithContext(ctx, false)
	if err != nil {
		return 0, fmt.Errorf("cpu times: %w", err)
	}
	if len(times) == 0 {
		return 0, nil
	}
	h.cpuMu.Lock()
	defer h.cpuMu.Unlock()
	cur := cpuSample{busy: cpuBusy(times[0]), total: cpuTotal(times[0])}
	prev := h.cpuLast
	h.cpuLast = cur
	return busyPercent(prev, cur), nil
}

// cpuSample is a cumulative CPU time reading in seconds.
type cpuSample struct{ busy, total float64 }

// cpuTotal is every CPU state (guest time is already part of user time).
func cpuTotal(t cpu.TimesStat) float64 {
	return t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
}

func cpuBusy(t cpu.TimesStat) float64 { return cpuTotal(t) - t.Idle - t.Iowait }

// busyPercent is the busy share between two readings, clamped to 0..100;
// 0 when no time passed.
func busyPercent(prev, cur cpuSample) float64 {
	total := cur.total - prev.total
	if total <= 0 {
		return 0
	}
	return math.Max(0, math.Min(100, (cur.busy-prev.busy)/total*100))
}

// cpuSensorPreference orders the sensor keys taken as "the" CPU temperature.
var cpuSensorPreference = []string{"package", "tctl", "tdie", "x86_pkg_temp", "cpu_thermal", "coretemp", "k10temp", "cpu", "acpitz"}

// CPUTemperature implements Source.
func (h *HostSource) CPUTemperature(ctx context.Context) *float64 {
	temps, _ := sensors.TemperaturesWithContext(ctx)
	for _, want := range cpuSensorPreference {
		for _, t := range temps {
			if strings.Contains(strings.ToLower(t.SensorKey), want) && t.Temperature > 0 {
				v := t.Temperature
				return &v
			}
		}
	}
	return nil
}

// Mem implements Source.
func (h *HostSource) Mem(ctx context.Context) (MemInfo, error) {
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return MemInfo{}, fmt.Errorf("virtual memory: %w", err)
	}
	m := MemInfo{Total: float64(vm.Total), Free: float64(vm.Free), Used: float64(vm.Total) - float64(vm.Free), Available: float64(vm.Available)}
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		m.SwapTotal = float64(sw.Total)
		m.SwapUsed = float64(sw.Total) - float64(sw.Free)
	}
	return m, nil
}

// NetworkStats implements Source: counters of the default interface and
// their rate since the previous call.
func (h *HostSource) NetworkStats(ctx context.Context) []NetStat {
	counters, err := psnet.IOCountersWithContext(ctx, true)
	if err != nil || len(counters) == 0 {
		return nil
	}
	iface := defaultInterface()
	pick := counters[0]
	for _, c := range counters {
		if c.Name == iface {
			pick = c
			break
		}
	}
	return []NetStat{h.speed(pick.Name, float64(pick.BytesRecv), float64(pick.BytesSent), time.Now())}
}

// speed computes network speed: rates are nil on the first
// sample of an interface and 0 when a counter went backwards.
func (h *HostSource) speed(iface string, rx, tx float64, now time.Time) NetStat {
	h.netMu.Lock()
	defer h.netMu.Unlock()
	st := NetStat{RxBytes: rx, TxBytes: tx}
	if last, ok := h.netLast[iface]; ok {
		secs := now.Sub(last.at).Seconds()
		rate := func(cur, prev float64) *float64 {
			v := 0.0
			if cur-prev >= 0 && secs > 0 {
				v = (cur - prev) / secs
			}
			return &v
		}
		st.RxSec, st.TxSec = rate(rx, last.rx), rate(tx, last.tx)
	}
	h.netLast[iface] = netSample{rx: rx, tx: tx, at: now}
	return st
}

// OSInfo implements Source.
func (h *HostSource) OSInfo(ctx context.Context) OSInfo {
	info := OSInfo{Platform: runtime.GOOS, Arch: NodeArch(runtime.GOARCH), Distro: distro(ctx)}
	if runtime.GOOS == "windows" {
		info.Platform = "Windows"
	}
	info.Hostname, _ = os.Hostname()
	kernel, _ := host.KernelVersionWithContext(ctx)
	info.Kernel = NodeRelease(runtime.GOOS, kernel)
	return info
}

// NodeRelease is os.release() from gopsutil's kernel version: unchanged on
// Unix (uname -r), "major.minor.build" on Windows, where gopsutil appends the
// update revision and build ("10.0.19045.6466 Build 19045.6466").
func NodeRelease(goos, kernel string) string {
	if goos != "windows" {
		return kernel
	}
	fields := strings.Fields(kernel)
	if len(fields) == 0 {
		return kernel
	}
	parts := strings.Split(fields[0], ".")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, ".")
}

// LoadAvg implements Source. Zeros are reported on Windows.
func (h *HostSource) LoadAvg(ctx context.Context) [3]float64 {
	if runtime.GOOS == "windows" {
		return [3]float64{}
	}
	avg, err := load.AvgWithContext(ctx)
	if err != nil {
		return [3]float64{}
	}
	return [3]float64{avg.Load1, avg.Load5, avg.Load15}
}

// FirstIPv4 implements Source: the first IPv4 address of an interface that
// is up and not a loopback.
func (h *HostSource) FirstIPv4(context.Context) string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				if v4 := ipn.IP.To4(); v4 != nil {
					return v4.String()
				}
			}
		}
	}
	return ""
}

// ReadHostFile implements Source.
func (h *HostSource) ReadHostFile(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

// Processes implements Source with the local process table.
func (h *HostSource) Processes(ctx context.Context) []RawProcess {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil
	}
	out := make([]RawProcess, 0, len(procs))
	for _, p := range procs {
		name, err := p.NameWithContext(ctx)
		if err != nil {
			continue
		}
		cpuPct, _ := p.CPUPercentWithContext(ctx)
		memPct, _ := p.MemoryPercentWithContext(ctx)
		user, _ := p.UsernameWithContext(ctx)
		out = append(out, RawProcess{PID: int(p.Pid), Name: name, CPU: cpuPct, Mem: float64(memPct), User: user})
	}
	return out
}

// Uptime implements Source.
func (h *HostSource) Uptime(ctx context.Context) float64 {
	if v, ok := procUptime(); ok {
		return v
	}
	up, err := host.UptimeWithContext(ctx)
	if err != nil {
		return 0
	}
	return float64(up)
}

// FsSize implements Source.
func (h *HostSource) FsSize(ctx context.Context) []FsEntry { return fsSize(ctx) }

// GPU implements Source.
func (h *HostSource) GPU(ctx context.Context) *GPUController { return gpu(ctx) }

// HostProcesses implements Source.
func (h *HostSource) HostProcesses(ctx context.Context) []Process { return hostProcesses(ctx) }

// EstablishedConnections implements Source.
func (h *HostSource) EstablishedConnections(ctx context.Context) (int, error) {
	return establishedConnections(ctx)
}
