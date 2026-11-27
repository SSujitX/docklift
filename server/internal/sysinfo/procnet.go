package sysinfo

import (
	"strings"
)

// ParseDefaultRoute returns the interface of the default route in
// /proc/net/route ("" when there is none).
func ParseDefaultRoute(content string) string {
	for i, line := range strings.Split(content, "\n") {
		if i == 0 {
			continue
		}
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] == "00000000" {
			return f[0]
		}
	}
	return ""
}

// CountEstablished counts sockets in state 01 (TCP_ESTABLISHED; connected
// UDP sockets use the same value) in /proc/net/{tcp,tcp6,udp,udp6} content.
func CountEstablished(content string) int {
	n := 0
	for i, line := range strings.Split(content, "\n") {
		if i == 0 {
			continue
		}
		f := strings.Fields(line)
		if len(f) > 3 && f[3] == "01" {
			n++
		}
	}
	return n
}

// ParseNvidiaSMI parses the first line of `nvidia-smi
// --query-gpu=name,memory.total,memory.used,temperature.gpu,utilization.gpu
// --format=csv,noheader,nounits`; nil when it is not such a line.
func ParseNvidiaSMI(out string) *GPUController {
	line, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	parts := strings.Split(line, ",")
	if len(parts) < 5 || strings.TrimSpace(parts[0]) == "" {
		return nil
	}
	num := func(s string) float64 {
		v := ParseFloat(strings.TrimSpace(s))
		if v != v {
			return 0
		}
		return v
	}
	return &GPUController{Model: strings.TrimSpace(parts[0]), VRAM: num(parts[1]), MemoryUsed: num(parts[2]),
		TemperatureGPU: num(parts[3]), UtilizationGPU: num(parts[4])}
}

// ParseProcUptime reads the first field of /proc/uptime.
func ParseProcUptime(content string) (float64, bool) {
	f := strings.Fields(content)
	if len(f) == 0 {
		return 0, false
	}
	v := ParseFloat(f[0])
	if v != v {
		return 0, false
	}
	return v, true
}
