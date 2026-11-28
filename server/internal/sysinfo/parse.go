package sysinfo

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"docklift/internal/js"
)

// HostOS is readHostOSInfo: the host's /etc/os-release mounted at /host/os-release.
type HostOS struct {
	Platform string
	Distro   string
	Release  string
}

// ParseOSRelease parses os-release content the way 2.x did: each line
// is split on "=" and only the first two parts are kept (a value containing
// "=" is cut there), double quotes are removed.
func ParseOSRelease(content string) HostOS {
	info := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		parts := strings.Split(line, "=")
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			continue
		}
		info[parts[0]] = strings.ReplaceAll(parts[1], `"`, "")
	}
	distro := info["PRETTY_NAME"]
	if distro == "" {
		distro = info["NAME"]
	}
	if distro == "" {
		distro = "Linux"
	}
	return HostOS{Platform: "linux", Distro: distro, Release: info["VERSION_ID"]}
}

// HostCPU is readHostCpuInfo: model, speed in GHz (one decimal) and logical
// processor count from /host/proc/cpuinfo. Zero values mean "not found".
type HostCPU struct {
	Model string
	Speed float64
	Cores int
}

func afterColon(line string) string {
	parts := strings.Split(line, ":")
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// ParseCPUInfo parses /proc/cpuinfo; the last "model name" and the last
// positive "cpu MHz" win, every "processor" line counts one core.
func ParseCPUInfo(content string) HostCPU {
	var c HostCPU
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "model name") {
			c.Model = afterColon(line)
		}
		if strings.Contains(strings.ToLower(line), "cpu mhz") {
			if mhz := js.ParseFloat(afterColon(line)); mhz > 0 {
				c.Speed = js.RoundHalfUp(mhz/100) / 10
			}
		}
		if strings.HasPrefix(line, "processor") {
			c.Cores++
		}
	}
	return c
}

// HostMem is readHostMemInfo, in bytes. NaN marks a value that did not parse
// (parseInt yielded NaN); the callers' `|| 0` turns it into zero.
type HostMem struct {
	Total, Free, Available, Buffers, Cached float64
}

// ParseMemInfo parses /proc/meminfo ("Key:   123 kB" lines) into bytes.
func ParseMemInfo(content string) HostMem {
	values := map[string]float64{}
	for _, line := range strings.Split(content, "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			continue
		}
		kb := js.ParseInt(strings.Split(strings.TrimSpace(parts[1]), " ")[0])
		values[parts[0]] = kb * 1024
	}
	orZero := func(k string) float64 {
		v := values[k]
		if math.IsNaN(v) {
			return 0
		}
		return v
	}
	return HostMem{
		Total:     orZero("MemTotal"),
		Free:      orZero("MemFree"),
		Available: orZero("MemAvailable"),
		Buffers:   orZero("Buffers"),
		Cached:    orZero("Cached"),
	}
}

// Process is one row of the process table.
type Process struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
	CPU  js.Num `json:"cpu"`
	Mem  js.Num `json:"mem"`
	User string `json:"user"`
}

var whitespace = regexp.MustCompile(`\s+`)

func firstLines(out string, n int) []string {
	lines := strings.Split(out, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return lines
}

// ParsePSAux parses `ps aux --sort=-%cpu | head -n 15` (USER PID %CPU %MEM
// VSZ RSS TTY STAT START TIME COMMAND); the command is cut to 50 UTF-16 units.
func ParsePSAux(out string) []Process {
	procs := []Process{}
	lines := firstLines(out, 15)
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		parts := whitespace.Split(line, -1)
		pid := js.ParseInt(field(parts, 1))
		name := ""
		if len(parts) > 10 {
			name = strings.Join(parts[10:], " ")
		}
		if math.IsNaN(pid) || name == "" {
			continue
		}
		procs = append(procs, Process{PID: int(pid), Name: js.Substring(name, 50),
			CPU: js.Num(js.ParseFloat(field(parts, 2))), Mem: js.Num(js.ParseFloat(field(parts, 3))), User: parts[0]})
	}
	return procs
}

// ParseTop parses `top -b -n 1 -w 512 -o %CPU | head -n 20` (PID USER PR NI
// VIRT RES SHR S %CPU %MEM TIME+ COMMAND), skipping top itself. It returns
// nil when the header line is missing.
func ParseTop(out string) []Process {
	lines := firstLines(out, 20)
	header := -1
	for i, l := range lines {
		if strings.Contains(l, "PID") && strings.Contains(l, "COMMAND") {
			header = i
			break
		}
	}
	if header == -1 {
		return nil
	}
	procs := []Process{}
	for i := header + 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		parts := whitespace.Split(line, -1)
		pid := js.ParseInt(parts[0])
		if math.IsNaN(pid) {
			continue
		}
		name := ""
		if len(parts) > 11 {
			name = strings.Join(parts[11:], " ")
		}
		if name == "top" {
			continue
		}
		procs = append(procs, Process{PID: int(pid), Name: name, CPU: js.Num(js.ParseFloat(field(parts, 8))),
			Mem: js.Num(js.ParseFloat(field(parts, 9))), User: field(parts, 1)})
	}
	return procs
}

func field(parts []string, i int) string {
	if i < len(parts) {
		return parts[i]
	}
	return ""
}

// FsEntry is one filesystem entry.
type FsEntry struct {
	FS        string
	Type      string
	Size      float64
	Used      float64
	Available float64
	Use       float64
	Mount     string
}

var dfWithType = regexp.MustCompile(`^(.+?)\s+(\S+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(?:\d+%|-)\s+(.+)$`)

var linuxTmpFileSystems = []string{"rootfs", "unionfs", "squashfs", "cramfs", "initrd", "initramfs", "devtmpfs", "tmpfs", "udev", "devfs", "specfs", "type", "appimaged"}

func isLinuxTmpFs(fsType string) bool {
	lower := strings.ToLower(fsType)
	for _, t := range linuxTmpFileSystems {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

// ParseDF parses `df -kPT` output the way 2.x did on Linux:
// header lines are dropped, real devices, the root mount and non-tmpfs types
// are kept, duplicates (same fs, type and mount) are listed once.
func ParseDF(out string) []FsEntry {
	lines := strings.Split(out, "\n")
	if len(lines) > 0 {
		lines = lines[1:]
	}
	if strings.Contains(strings.ToLower(out), "filesystem") {
		remove := 0
		for i, l := range lines {
			if l != "" && strings.HasPrefix(strings.ToLower(l), "filesystem") {
				remove = i
			}
		}
		lines = lines[remove:]
	}
	data := []FsEntry{}
	for _, line := range lines {
		if line == "" {
			continue
		}
		m := dfWithType.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		fs, fsType, mount := m[1], m[2], m[6]
		if !(strings.HasPrefix(fs, "/") || mount == "/" || strings.Index(fs, "/") > 0 || strings.Index(fs, ":") == 1 || !isLinuxTmpFs(fsType)) {
			continue
		}
		size := js.ParseInt(m[3]) * 1024
		used := js.ParseInt(m[4]) * 1024
		available := js.ParseInt(m[5]) * 1024
		dup := false
		for _, d := range data {
			if d.FS == fs && d.Type == fsType && d.Mount == mount {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		data = append(data, FsEntry{FS: fs, Type: fsType, Size: size, Used: used, Available: available,
			Use: toFixed(100.0*(used/(used+available)), 2), Mount: mount})
	}
	return data
}

// FormatUptime is formatUptime: "2 Days, 3 Hours, 1 Min", or "< 1 Min".
func FormatUptime(seconds float64) string {
	days := math.Floor(seconds / 86400)
	hours := math.Floor(math.Mod(seconds, 86400) / 3600)
	minutes := math.Floor(math.Mod(seconds, 3600) / 60)
	plural := func(n float64, unit string) string {
		if n > 1 {
			return fmt.Sprintf("%s %ss", js.NumberString(n), unit)
		}
		return fmt.Sprintf("%s %s", js.NumberString(n), unit)
	}
	var parts []string
	if days > 0 {
		parts = append(parts, plural(days, "Day"))
	}
	if hours > 0 {
		parts = append(parts, plural(hours, "Hour"))
	}
	if minutes > 0 {
		parts = append(parts, plural(minutes, "Min"))
	}
	if len(parts) == 0 {
		return "< 1 Min"
	}
	return strings.Join(parts, ", ")
}

var brandReplacements = []struct {
	re   *regexp.Regexp
	with string
}{
	{regexp.MustCompile(`\(R\)+`), "®"},
	{regexp.MustCompile(`\(TM\)+`), "™"},
	{regexp.MustCompile(`\(C\)+`), "©"},
	{regexp.MustCompile(`CPU+`), ""},
}

// CPUBrand cleans up a CPU model line: the part
// before "@", (R)/(TM)/(C) turned into symbols, "CPU" removed and the vendor
// (first word) dropped. "Intel(R) Core(TM) i7-8700 CPU @ 3.20GHz" becomes
// "Core™ i7-8700".
func CPUBrand(model string) string {
	b, _, _ := strings.Cut(model, "@")
	b = strings.TrimSpace(b)
	for _, r := range brandReplacements {
		b = r.re.ReplaceAllString(b, r.with)
		b = strings.TrimSpace(whitespace.ReplaceAllString(b, " "))
	}
	parts := strings.Split(b, " ")
	return strings.Join(parts[1:], " ")
}

// CPUSpeedFromModel is the GHz value after "@" in a model line (parseFloat),
// 0 when there is none.
func CPUSpeedFromModel(model string) float64 {
	_, after, ok := strings.Cut(model, "@")
	if !ok {
		return 0
	}
	v := js.ParseFloat(strings.TrimSpace(after))
	if math.IsNaN(v) {
		return 0
	}
	return v
}

// LocaleString is JavaScript's new Date().toLocaleString() under the default en-US
// ICU locale: "11/27/2026, 3:04:05 PM" in local time. ICU 72+ formats a
// narrow no-break space before the day period, but V8 (as used by Docklift 2.x)
// replaces it with a plain space; the
// live 2.x server was checked to answer with U+0020.
func LocaleString(t time.Time) string {
	return t.Format("1/2/2006, 3:04:05 PM")
}

// ISOString is Date.prototype.toISOString: UTC with milliseconds.
func ISOString(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// NodeArch is os.arch() for the running binary.
func NodeArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "x64"
	case "386":
		return "ia32"
	case "ppc64le":
		return "ppc64"
	case "mipsle":
		return "mipsel"
	}
	return goarch
}

// toFixed is parseFloat(x.toFixed(digits)).
func toFixed(x float64, digits int) float64 {
	return js.ParseFloat(js.ToFixed(x, digits))
}
