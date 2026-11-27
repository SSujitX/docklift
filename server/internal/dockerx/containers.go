package dockerx

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// ContainerStatus is the {status, running} pair getContainerStatus returned.
// Status is the Docker state ("running", "exited", "dead", ...) or
// "not_found".
type ContainerStatus struct {
	Status  string `json:"status"`
	Running bool   `json:"running"`
}

// StatusNotFound is the Status of a container that does not exist (or when
// Docker cannot be reached, exactly like the TS code).
const StatusNotFound = "not_found"

// ContainerStatus inspects a container by exact name and falls back to a
// name-filtered list (Docker's filter is a substring match). Every failure,
// including an unreachable daemon, yields {not_found, false}.
func (d *Docker) ContainerStatus(ctx context.Context, name string) ContainerStatus {
	if d.Engine == nil {
		return ContainerStatus{Status: StatusNotFound}
	}
	if res, err := d.Engine.ContainerInspect(ctx, name, client.ContainerInspectOptions{}); err == nil && res.Container.State != nil {
		return ContainerStatus{Status: string(res.Container.State.Status), Running: res.Container.State.Running}
	}
	list, err := d.Engine.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: make(client.Filters).Add("name", name)})
	if err == nil && len(list.Items) > 0 {
		state := string(list.Items[0].State)
		return ContainerStatus{Status: state, Running: state == "running"}
	}
	return ContainerStatus{Status: StatusNotFound}
}

// ContainerImageRef returns the image reference a container was created
// from (Config.Image, else the image id), or "" when it cannot be inspected.
func (d *Docker) ContainerImageRef(ctx context.Context, name string) string {
	if d.Engine == nil || strings.TrimSpace(name) == "" {
		return ""
	}
	res, err := d.Engine.ContainerInspect(ctx, name, client.ContainerInspectOptions{})
	if err != nil {
		return ""
	}
	if res.Container.Config != nil && res.Container.Config.Image != "" {
		return res.Container.Config.Image
	}
	return res.Container.Image
}

// ContainerLogs returns the last tail lines of stdout+stderr without
// timestamps, or "" on any error (getContainerLogs). Multiplexed (non-TTY)
// output is demultiplexed into plain text.
func (d *Docker) ContainerLogs(ctx context.Context, name string, tail int) string {
	if d.Engine == nil {
		return ""
	}
	tty := false
	if res, err := d.Engine.ContainerInspect(ctx, name, client.ContainerInspectOptions{}); err == nil && res.Container.Config != nil {
		tty = res.Container.Config.Tty
	}
	rc, err := d.Engine.ContainerLogs(ctx, name, client.ContainerLogsOptions{
		ShowStdout: true, ShowStderr: true, Tail: strconv.Itoa(tail),
	})
	if err != nil {
		return ""
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return ""
	}
	if tty {
		return string(raw)
	}
	var out strings.Builder
	for _, f := range demuxFrames(&raw) {
		out.Write(f)
	}
	out.Write(raw)
	return out.String()
}

// demuxFrames consumes complete Docker stream frames ([type][0 0 0][size BE
// uint32][payload]) from the front of *buf and returns their payloads;
// an incomplete trailing frame stays in *buf.
func demuxFrames(buf *[]byte) [][]byte {
	var frames [][]byte
	b := *buf
	for len(b) >= 8 {
		size := int(binary.BigEndian.Uint32(b[4:8]))
		if len(b) < 8+size {
			break
		}
		frames = append(frames, b[8:8+size])
		b = b[8+size:]
	}
	*buf = b
	return frames
}

// ContainerStats is the formatted one-shot stats object of
// getContainerStats (all values are strings, like the TS toFixed output).
type ContainerStats struct {
	CPUPercent    string `json:"cpu_percent"`
	MemoryUsage   string `json:"memory_usage"`
	MemoryLimit   string `json:"memory_limit"`
	MemoryPercent string `json:"memory_percent"`
}

// ContainerStats samples a container once (with the previous CPU sample, as
// dockerode's stream:false did) and formats it; nil means the stats could
// not be read (the TS null).
func (d *Docker) ContainerStats(ctx context.Context, name string) *ContainerStats {
	if d.Engine == nil {
		return nil
	}
	res, err := d.Engine.ContainerStats(ctx, name, client.ContainerStatsOptions{Stream: false, IncludePreviousSample: true})
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	var s container.StatsResponse
	if err := json.NewDecoder(res.Body).Decode(&s); err != nil {
		return nil
	}
	return FormatStats(&s)
}

// FormatStats computes the CPU and memory figures exactly like
// getContainerStats in docker.ts.
func FormatStats(s *container.StatsResponse) *ContainerStats {
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(s.CPUStats.SystemUsage) - float64(s.PreCPUStats.SystemUsage)
	onlineCPUs := float64(s.CPUStats.OnlineCPUs)
	if onlineCPUs == 0 {
		onlineCPUs = float64(len(s.CPUStats.CPUUsage.PercpuUsage))
	}
	if onlineCPUs == 0 {
		onlineCPUs = 1
	}
	cpuPercent := 0.0
	if systemDelta > 0 {
		cpuPercent = cpuDelta / systemDelta * onlineCPUs * 100
	}
	memoryUsage := float64(s.MemoryStats.Usage)
	memoryLimit := float64(s.MemoryStats.Limit)
	if memoryLimit == 0 {
		memoryLimit = 1
	}
	memoryPercent := memoryUsage / memoryLimit * 100
	finite := func(v float64) float64 {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0
		}
		return v
	}
	return &ContainerStats{
		CPUPercent:    ToFixed(finite(cpuPercent), 2),
		MemoryUsage:   ToFixed(memoryUsage/1024/1024, 2) + " MB",
		MemoryLimit:   ToFixed(memoryLimit/1024/1024, 2) + " MB",
		MemoryPercent: ToFixed(finite(memoryPercent), 2),
	}
}

// ToFixed formats v like JavaScript Number.prototype.toFixed(digits): the
// exact binary value is rounded half away from zero (Go's 'f' formatting
// rounds exact ties to even, e.g. 0.125 -> "0.12" where JS gives "0.13").
// Values of magnitude >= 1e21 use JS's exponent form, as toFixed does.
func ToFixed(v float64, digits int) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "Infinity"
	case math.IsInf(v, -1):
		return "-Infinity"
	case math.Abs(v) >= 1e21:
		return jsExponent(v)
	}
	r := new(big.Rat).SetFloat64(v)
	neg := r.Sign() < 0
	r.Abs(r)
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
	r.Mul(r, new(big.Rat).SetInt(scale))
	// n = floor(r + 1/2): ties go up (away from zero after the sign is restored).
	r.Add(r, big.NewRat(1, 2))
	n := new(big.Int).Quo(r.Num(), r.Denom())
	s := n.String()
	if digits > 0 {
		if len(s) <= digits {
			s = strings.Repeat("0", digits-len(s)+1) + s
		}
		s = s[:len(s)-digits] + "." + s[len(s)-digits:]
	}
	if neg {
		// JS keeps the sign even when the value rounds to zero: (-0.001).toFixed(2) === "-0.00".
		s = "-" + s
	}
	return s
}

// jsExponent formats like Number.prototype.toString for large magnitudes.
func jsExponent(v float64) string {
	s := strconv.FormatFloat(v, 'e', -1, 64)
	mant, exp, _ := strings.Cut(s, "e")
	n, _ := strconv.Atoi(exp)
	sign := "+"
	if n < 0 {
		sign = "-"
		n = -n
	}
	return fmt.Sprintf("%se%s%d", mant, sign, n)
}
