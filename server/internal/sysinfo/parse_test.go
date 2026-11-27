package sysinfo

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestToFixed(t *testing.T) {
	cases := []struct {
		x      float64
		digits int
		want   float64
	}{
		{0.25, 1, 0.3}, // JS rounds exact ties up; strconv would give 0.2
		{0.35, 1, 0.3}, // 0.35 is 0.34999… in binary
		{1.005, 2, 1},  // 1.00499…
		{2.5, 0, 3},
		{-2.5, 0, -3},
		{12.345678, 1, 12.3},
		{99.95, 1, 100}, // 99.9500000000000028…
		{0, 1, 0},
		{45.6789, 2, 45.68},
	}
	for _, c := range cases {
		if got := ToFixed(c.x, c.digits); got != c.want {
			t.Errorf("ToFixed(%v, %d) = %v, want %v", c.x, c.digits, got, c.want)
		}
	}
	if !math.IsNaN(ToFixed(math.NaN(), 1)) {
		t.Error("NaN must stay NaN")
	}
}

func TestParseFloatAndInt(t *testing.T) {
	floats := map[string]float64{"3.20GHz": 3.2, "  2400.000": 2400, "-1.5e3x": -1500, ".5": 0.5, "Infinity": math.Inf(1)}
	for in, want := range floats {
		if got := ParseFloat(in); got != want {
			t.Errorf("ParseFloat(%q) = %v, want %v", in, got, want)
		}
	}
	for _, in := range []string{"", "abc", "-", "."} {
		if !math.IsNaN(ParseFloat(in)) {
			t.Errorf("ParseFloat(%q) must be NaN", in)
		}
	}
	ints := map[string]float64{"16318412": 16318412, " 42 kB": 42, "-7": -7, "0x1A": 26, "12.9": 12, "77abc": 77}
	for in, want := range ints {
		if got := ParseInt(in); got != want {
			t.Errorf("ParseInt(%q) = %v, want %v", in, got, want)
		}
	}
	if !math.IsNaN(ParseInt("kB")) {
		t.Error("ParseInt without digits must be NaN")
	}
}

func TestSubstringCountsUTF16Units(t *testing.T) {
	if got := Substring("abcdef", 3); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := Substring("a😀b", 2); got != "a" { // the emoji is two units
		t.Fatalf("got %q", got)
	}
	if got := Substring("short", 50); got != "short" {
		t.Fatalf("got %q", got)
	}
}

func TestNumJSON(t *testing.T) {
	b, _ := json.Marshal([]Num{Num(math.NaN()), Num(math.Inf(1)), 12, 1.5, 16710533120})
	if string(b) != "[null,null,12,1.5,16710533120]" {
		t.Fatalf("got %s", b)
	}
}

func TestParseOSRelease(t *testing.T) {
	got := ParseOSRelease("NAME=\"Ubuntu\"\nVERSION_ID=\"24.04\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\nBROKEN\nX=a=b")
	if got != (HostOS{Platform: "linux", Distro: "Ubuntu 24.04.1 LTS", Release: "24.04"}) {
		t.Fatalf("got %+v", got)
	}
	if d := ParseOSRelease("NAME=Debian").Distro; d != "Debian" {
		t.Fatalf("NAME fallback: %q", d)
	}
	if d := ParseOSRelease("ID=x").Distro; d != "Linux" {
		t.Fatalf("default: %q", d)
	}
}

func TestParseCPUInfo(t *testing.T) {
	content := "processor\t: 0\nmodel name\t: Intel(R) Xeon(R) CPU @ 2.20GHz\ncpu MHz\t\t: 2199.998\n\nprocessor\t: 1\nmodel name\t: Intel(R) Xeon(R) CPU @ 2.20GHz\ncpu MHz\t\t: 2250.000"
	got := ParseCPUInfo(content)
	if got != (HostCPU{Model: "Intel(R) Xeon(R) CPU @ 2.20GHz", Speed: 2.3, Cores: 2}) {
		t.Fatalf("got %+v", got)
	}
	arm := ParseCPUInfo("processor\t: 0\nBogoMIPS\t: 48.00\nprocessor\t: 1")
	if arm != (HostCPU{Cores: 2}) {
		t.Fatalf("arm: %+v", arm)
	}
}

func TestParseMemInfo(t *testing.T) {
	got := ParseMemInfo("MemTotal:       16318412 kB\nMemFree:         1000000 kB\nMemAvailable:    8000000 kB\nBuffers:          100 kB\nCached:  x kB")
	want := HostMem{Total: 16318412 * 1024, Free: 1000000 * 1024, Available: 8000000 * 1024, Buffers: 100 * 1024, Cached: 0}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParsePSAux(t *testing.T) {
	out := "USER         PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND\n" +
		"root           1  2.5  0.1 168000 12000 ?        Ss   Nov01   1:23 /sbin/init splash\n" +
		"www-data    4242 10.0  3.2 900000 50000 ?        Sl   10:00   0:42 /usr/bin/a-very-long-command-name --with --many --arguments --that --go --on\n" +
		"broken line\n"
	got := ParsePSAux(out)
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got[0] != (Process{PID: 1, Name: "/sbin/init splash", CPU: 2.5, Mem: 0.1, User: "root"}) {
		t.Fatalf("row 0: %+v", got[0])
	}
	if len(got[1].Name) != 50 || got[1].User != "www-data" || got[1].PID != 4242 {
		t.Fatalf("row 1: %+v", got[1])
	}
	var lines string
	for i := 0; i < 30; i++ {
		lines += "root 1 0.0 0.0 0 0 ? S 00:00 0:00 cmd\n"
	}
	if n := len(ParsePSAux("HEADER\n" + lines)); n != 14 {
		t.Fatalf("head -n 15 keeps 14 rows, got %d", n)
	}
	if got := ParsePSAux(""); got == nil || len(got) != 0 {
		t.Fatalf("empty output is an empty list, got %#v", got)
	}
}

func TestParseTop(t *testing.T) {
	out := "top - 10:00:00 up 1 day\nTasks: 3 total\n\n    PID USER      PR  NI    VIRT    RES    SHR S  %CPU  %MEM     TIME+ COMMAND\n" +
		"      1 root      20   0  168000  12000   9000 S   6.2   0.1   1:23.45 node dist/index.js\n" +
		"     99 root      20   0    1000    900    800 R   0.0   0.0   0:00.01 top\n"
	got := ParseTop(out)
	if len(got) != 1 || got[0] != (Process{PID: 1, Name: "node dist/index.js", CPU: 6.2, Mem: 0.1, User: "root"}) {
		t.Fatalf("got %+v", got)
	}
	if ParseTop("no header here") != nil {
		t.Fatal("missing header must yield nil")
	}
}

func TestParseDF(t *testing.T) {
	out := "Filesystem     Type     1024-blocks      Used Available Capacity Mounted on\n" +
		"overlay        overlay     102687672  40000000  57428184      42% /\n" +
		"tmpfs          tmpfs           65536         0     65536       0% /dev\n" +
		"shm            tmpfs           65536         0     65536       0% /dev/shm\n" +
		"/dev/sda1      ext4        102687672  40000000  57428184      42% /etc/hosts\n" +
		"/dev/sda1      ext4        102687672  40000000  57428184      42% /etc/hosts\n" +
		"/dev/sdb1      ext4        10000      5000      5000         50% /mnt/my data\n"
	got := ParseDF(out)
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	if got[0].FS != "overlay" || got[0].Mount != "/" || got[0].Size != 102687672*1024 || got[0].Use != 41.06 {
		t.Fatalf("root: %+v", got[0])
	}
	if got[2].Mount != "/mnt/my data" || got[2].Use != 50 {
		t.Fatalf("spaces in mount: %+v", got[2])
	}
}

func TestFormatUptime(t *testing.T) {
	cases := map[float64]string{
		30:                 "< 1 Min",
		60:                 "1 Min",
		3 * 60:             "3 Mins",
		3600 + 120:         "1 Hour, 2 Mins",
		2*86400 + 3*3600:   "2 Days, 3 Hours",
		86400 + 60 + 0.987: "1 Day, 1 Min",
	}
	for in, want := range cases {
		if got := FormatUptime(in); got != want {
			t.Errorf("FormatUptime(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestCPUBrandAndSpeed(t *testing.T) {
	if got := CPUBrand("Intel(R) Core(TM) i7-8700 CPU @ 3.20GHz"); got != "Core™ i7-8700" {
		t.Fatalf("got %q", got)
	}
	if got := CPUBrand("AMD Ryzen 7 5800X 8-Core Processor"); got != "Ryzen 7 5800X 8-Core Processor" {
		t.Fatalf("got %q", got)
	}
	if got := CPUSpeedFromModel("Intel(R) Core(TM) i7-8700 CPU @ 3.20GHz"); got != 3.2 {
		t.Fatalf("got %v", got)
	}
	if got := CPUSpeedFromModel("AMD Ryzen"); got != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestTimeFormats(t *testing.T) {
	ts := time.Date(2026, 11, 27, 15, 4, 5, 678_000_000, time.UTC)
	if got := LocaleString(ts); got != "11/27/2026, 3:04:05 PM" {
		t.Fatalf("LocaleString = %q", got)
	}
	if got := ISOString(ts); got != "2026-11-27T15:04:05.678Z" {
		t.Fatalf("ISOString = %q", got)
	}
	if NodeArch("amd64") != "x64" || NodeArch("arm64") != "arm64" || NodeArch("386") != "ia32" {
		t.Fatal("NodeArch mapping")
	}
}

func TestProcNetParsers(t *testing.T) {
	route := "Iface\tDestination\tGateway\nveth0\t0011A8C0\t00000000\neth0\t00000000\t0101A8C0\n"
	if got := ParseDefaultRoute(route); got != "eth0" {
		t.Fatalf("default route: %q", got)
	}
	tcp := "  sl  local_address rem_address   st tx_queue\n   0: 0100007F:1F90 00000000:0000 0A 00000000\n   1: 0100007F:1F90 0100007F:D2A4 01 00000000\n   2: 0100007F:1F90 0100007F:D2A5 01 00000000\n"
	if got := CountEstablished(tcp); got != 2 {
		t.Fatalf("established: %d", got)
	}
	if up, ok := ParseProcUptime("12345.67 54321.00\n"); !ok || up != 12345.67 {
		t.Fatalf("uptime: %v %v", up, ok)
	}
	if _, ok := ParseProcUptime(""); ok {
		t.Fatal("empty uptime")
	}
	g := ParseNvidiaSMI("NVIDIA GeForce RTX 3080, 10240, 1234, 45, 0\n")
	if g == nil || *g != (GPUController{Model: "NVIDIA GeForce RTX 3080", VRAM: 10240, MemoryUsed: 1234, TemperatureGPU: 45}) {
		t.Fatalf("nvidia: %+v", g)
	}
	if ParseNvidiaSMI("No devices were found") != nil {
		t.Fatal("no GPU line")
	}
}

func TestNetworkSpeed(t *testing.T) {
	h := NewHostSource()
	t0 := time.Now()
	first := h.speed("eth0", 1000, 500, t0)
	if first.RxSec != nil || first.TxSec != nil {
		t.Fatal("the first sample has no rate")
	}
	second := h.speed("eth0", 3000, 400, t0.Add(2*time.Second))
	if *second.RxSec != 1000 || *second.TxSec != 0 {
		t.Fatalf("rates %v %v", *second.RxSec, *second.TxSec)
	}
}

func TestHostSourceReadsThisMachine(t *testing.T) {
	h := NewHostSource()
	ctx := t.Context()
	m, err := h.Mem(ctx)
	if err != nil || m.Total <= 0 || m.Used != m.Total-m.Free {
		t.Fatalf("mem %+v %v", m, err)
	}
	c, err := h.CPU(ctx)
	if err != nil || c.Cores <= 0 {
		t.Fatalf("cpu %+v %v", c, err)
	}
	if _, err := h.CurrentLoad(ctx); err != nil {
		t.Fatal(err)
	}
	if o := h.OSInfo(ctx); o.Platform == "" || o.Arch == "" {
		t.Fatalf("os %+v", o)
	}
	if _, ok := h.ReadHostFile("/definitely/not/here"); ok {
		t.Fatal("missing host file must report false")
	}
}

func TestNodeRelease(t *testing.T) {
	cases := []struct{ goos, kernel, want string }{
		{"windows", "10.0.19045.6466 Build 19045.6466", "10.0.19045"},
		{"windows", "10.0.22631", "10.0.22631"},
		{"windows", "", ""},
		{"linux", "6.8.0-45-generic", "6.8.0-45-generic"},
		{"darwin", "23.1.0", "23.1.0"},
	}
	for _, c := range cases {
		if got := NodeRelease(c.goos, c.kernel); got != c.want {
			t.Errorf("NodeRelease(%q, %q) = %q, want %q", c.goos, c.kernel, got, c.want)
		}
	}
}

func TestBusyPercent(t *testing.T) {
	cases := []struct {
		prev, cur cpuSample
		want      float64
	}{
		{cpuSample{}, cpuSample{busy: 25, total: 100}, 25}, // first call: since boot
		{cpuSample{busy: 25, total: 100}, cpuSample{busy: 35, total: 120}, 50},
		{cpuSample{busy: 25, total: 100}, cpuSample{busy: 25, total: 100}, 0}, // no time passed
		{cpuSample{busy: 50, total: 100}, cpuSample{busy: 40, total: 110}, 0}, // counter went backwards
	}
	for _, c := range cases {
		if got := busyPercent(c.prev, c.cur); got != c.want {
			t.Errorf("busyPercent(%+v, %+v) = %v, want %v", c.prev, c.cur, got, c.want)
		}
	}
}
