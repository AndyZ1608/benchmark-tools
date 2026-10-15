package system

import (
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

type Interface struct {
	Name, State string
	SpeedMbps   int64
}
type group struct {
	path, mount string
	v2          bool
	controllers string
}

func unescape(s string) string {
	r := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return r.Replace(s)
}
func cpuCount(s string) int {
	n := 0
	for _, entry := range strings.Split(s, ",") {
		ends := strings.Split(entry, "-")
		a, e := strconv.Atoi(ends[0])
		if e != nil || a < 0 {
			return 0
		}
		b := a
		if len(ends) == 2 {
			b, e = strconv.Atoi(ends[1])
			if e != nil || b < a {
				return 0
			}
		} else if len(ends) != 1 {
			return 0
		}
		if b-a > 1<<20 || n > 1<<20 {
			return 0
		}
		n += b - a + 1
	}
	return n
}
func groups(read func(string) string) []group {
	var out []group
	for _, line := range strings.Split(read("/proc/self/mountinfo"), "\n") {
		halves := strings.SplitN(line, " - ", 2)
		if len(halves) != 2 {
			continue
		}
		a, b := strings.Fields(halves[0]), strings.Fields(halves[1])
		if len(a) < 5 || len(b) < 3 || (b[0] != "cgroup2" && b[0] != "cgroup") {
			continue
		}
		for _, entry := range strings.Split(read("/proc/self/cgroup"), "\n") {
			c := strings.SplitN(entry, ":", 3)
			if len(c) != 3 {
				continue
			}
			v2 := b[0] == "cgroup2"
			if v2 && c[1] != "" {
				continue
			}
			if !v2 {
				matched := false
				for _, controller := range strings.Split(c[1], ",") {
					if strings.Contains(","+b[2]+",", ","+controller+",") {
						matched = true
					}
				}
				if !matched {
					continue
				}
			}
			mountRoot, mount := path.Clean(unescape(a[3])), path.Clean(unescape(a[4]))
			member := path.Clean(c[2])
			relative := ""
			if member == "/" {
				relative = ""
			} else if mountRoot == "/" {
				relative = strings.TrimPrefix(member, "/")
			} else if member == mountRoot {
				relative = ""
			} else if strings.HasPrefix(member, mountRoot+"/") {
				relative = strings.TrimPrefix(member, mountRoot+"/")
			} else {
				continue
			}
			out = append(out, group{path.Join(mount, relative), mount, v2, c[1]})
		}
	}
	return out
}
func enrich(s *Info, root string) {
	read := func(path string) string { return Read(filepath.Join(root, path)) }
	// Recompute headroom from each restricted ancestor's own usage, not just leaf usage.
	for _, line := range strings.Split(read("/proc/meminfo"), "\n") {
		v := strings.Fields(line)
		if len(v) < 2 {
			continue
		}
		n, e := strconv.ParseInt(v[1], 10, 64)
		if e != nil || n < 0 || n > math.MaxInt64/1024 {
			continue
		}
		switch v[0] {
		case "MemAvailable:":
			s.AvailableMemory = n * 1024
		case "MemTotal:":
			s.TotalMemory = n * 1024
		}
	}
	usable := s.LogicalCPUs
	if online := cpuCount(read("/sys/devices/system/cpu/online")); online > 0 {
		s.LogicalCPUs = online
	}
	s.Capacity = float64(min(s.LogicalCPUs, usable))
	s.CPUQuota = "unknown"
	quotaCapacity := math.Inf(1)
	if n := cpuCount(s.Affinity); n > 0 {
		s.Capacity = math.Min(s.Capacity, float64(n))
	} else {
		s.DetectionWarnings = append(s.DetectionWarnings, "CPU affinity unavailable; worker ceiling uses visible CPUs")
	}
	resolvedGroups := groups(read)
	if len(resolvedGroups) == 0 {
		s.DetectionWarnings = append(s.DetectionWarnings, "Cgroup restrictions unavailable: no resolvable controller mount; using visible CPU affinity and guest memory headroom")
	}
	for _, g := range resolvedGroups {
		for groupPath := g.path; ; groupPath = path.Dir(groupPath) {
			path := groupPath
			if g.v2 || strings.Contains(g.controllers, "cpu") {
				q := strings.Fields(read(filepath.Join(path, "cpu.max")))
				if !g.v2 {
					q = []string{read(filepath.Join(path, "cpu.cfs_quota_us")), read(filepath.Join(path, "cpu.cfs_period_us"))}
				}
				if len(q) == 2 {
					a, e := strconv.ParseFloat(q[0], 64)
					b, f := strconv.ParseFloat(q[1], 64)
					if e == nil && f == nil && a > 0 && b > 0 && finiteQuota(a/b) {
						s.Capacity = math.Min(s.Capacity, a/b)
						if a/b < quotaCapacity {
							quotaCapacity = a / b
							s.CPUQuota = strings.Join(q, " ")
						}
					}
				}
				s.Raw[filepath.Join(path, "cpu.stat")] = read(filepath.Join(path, "cpu.stat"))
			}
			cpus := read(filepath.Join(path, "cpuset.cpus.effective"))
			if cpus == "unknown" {
				cpus = read(filepath.Join(path, "cpuset.cpus"))
			}
			if n := cpuCount(cpus); n > 0 {
				s.Capacity = math.Min(s.Capacity, float64(n))
			}
			limits := []string{"memory.max", "memory.high"}
			usage := "memory.current"
			if !g.v2 {
				limits = []string{"memory.limit_in_bytes"}
				usage = "memory.usage_in_bytes"
			}
			for _, limitName := range limits {
				limit, e := strconv.ParseInt(read(filepath.Join(path, limitName)), 10, 64)
				if e != nil || limit < 0 || limit >= 1<<60 {
					continue
				}
				if s.MemoryLimit == 0 || limit < s.MemoryLimit {
					s.MemoryLimit = limit
				}
				used, e := strconv.ParseInt(read(filepath.Join(path, usage)), 10, 64)
				if e != nil || used < 0 {
					s.AvailableMemory = 0
					s.DetectionWarnings = append(s.DetectionWarnings, "Restricted memory usage unavailable; automatic RAM allocation disabled")
					continue
				}
				headroom := limit - used
				if headroom < 0 {
					headroom = 0
				}
				if headroom < s.AvailableMemory {
					s.AvailableMemory = headroom
				}
			}
			if path == g.mount {
				break
			}
		}
	}
	for _, line := range strings.Split(read("/proc/cpuinfo"), "\n") {
		p := strings.SplitN(line, ":", 2)
		if len(p) == 2 && (strings.TrimSpace(p[0]) == "flags" || strings.TrimSpace(p[0]) == "Features") {
			s.CPUFlags = strings.TrimSpace(p[1])
			break
		}
	}
	s.MemoryPressure = read("/proc/pressure/memory")
	for _, line := range strings.Split(read("/proc/vmstat"), "\n") {
		v := strings.Fields(line)
		if len(v) != 2 {
			continue
		}
		n, _ := strconv.ParseUint(v[1], 10, 64)
		if v[0] == "pswpin" {
			s.SwapIn = n
		}
		if v[0] == "pswpout" {
			s.SwapOut = n
		}
	}
	// Count unique last-level cache sharing domains across all visible CPU entries.
	paths, _ := filepath.Glob(filepath.Join(root, "/sys/devices/system/cpu/cpu[0-9]*/cache/index*"))
	maxLevel := 0
	domains := map[string]int64{}
	for _, path := range paths {
		relative := strings.TrimPrefix(path, root)
		level, _ := strconv.Atoi(read(filepath.Join(relative, "level")))
		kind := read(filepath.Join(relative, "type"))
		if kind != "Unified" && kind != "Data" {
			continue
		}
		if level > maxLevel {
			maxLevel = level
			domains = map[string]int64{}
		}
		if level != maxLevel {
			continue
		}
		size := read(filepath.Join(relative, "size"))
		n, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimSuffix(size, "K"), "M"), 10, 64)
		if strings.HasSuffix(size, "K") {
			n *= 1024
		}
		if strings.HasSuffix(size, "M") {
			n *= 1 << 20
		}
		share := read(filepath.Join(relative, "shared_cpu_list"))
		if share == "unknown" {
			share = path
		}
		domains[share] = n
	}
	for _, n := range domains {
		if n > 0 && s.LastLevelCache < math.MaxInt64-n {
			s.LastLevelCache += n
		}
	}
	nodes, _ := filepath.Glob(filepath.Join(root, "/sys/devices/system/node/node[0-9]*"))
	s.NUMANodes = len(nodes)
	entries, _ := os.ReadDir(filepath.Join(root, "/sys/class/net"))
	for _, e := range entries {
		name := e.Name()
		speed, _ := strconv.ParseInt(read("/sys/class/net/"+name+"/speed"), 10, 64)
		if speed < 0 {
			speed = 0
		}
		s.Interfaces = append(s.Interfaces, Interface{name, read("/sys/class/net/" + name + "/operstate"), speed})
	}
	if s.LastLevelCache == 0 {
		s.DetectionWarnings = append(s.DetectionWarnings, "Aggregate last-level cache size unavailable")
	}
	if s.NUMANodes == 0 {
		s.DetectionWarnings = append(s.DetectionWarnings, "NUMA topology unavailable")
	}
	if s.CPUFlags == "" {
		s.DetectionWarnings = append(s.DetectionWarnings, "CPU instruction capabilities unavailable")
	}
	if s.Capacity <= 0 {
		s.Capacity = 1
		s.DetectionWarnings = append(s.DetectionWarnings, fmt.Sprint("CPU capacity unknown; using one worker"))
	}
}
func finiteQuota(n float64) bool { return !math.IsInf(n, 0) && !math.IsNaN(n) }

// Throttling reads the process's resolved cgroup hierarchy, including parent limits.
func Throttling() string {
	var lines []string
	for _, g := range groups(Read) {
		for p := g.path; ; p = path.Dir(p) {
			value := Read(filepath.Join(p, "cpu.stat"))
			if value != "unknown" {
				lines = append(lines, p+"\n"+value)
			}
			if p == g.mount {
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}
