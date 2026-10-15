package system

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type Info struct {
	Hostname, OS, Kernel, Architecture, CPUModel, Affinity, CPUQuota, Hypervisor string
	LogicalCPUs                                                                  int
	Capacity                                                                     float64
	AvailableMemory, MemoryLimit                                                 int64
	TotalMemory, LastLevelCache                                                  int64
	NUMANodes                                                                    int
	CPUFlags, MemoryPressure                                                     string
	SwapIn, SwapOut                                                              uint64
	Interfaces                                                                   []Interface
	DetectionWarnings                                                            []string
	Raw                                                                          map[string]string
}

func Read(p string) string {
	b, e := os.ReadFile(p)
	if e != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(b))
}
func Detect() Info {
	h, _ := os.Hostname()
	s := Info{Hostname: h, OS: Read("/etc/os-release"), Kernel: Read("/proc/sys/kernel/osrelease"), Architecture: runtime.GOARCH, LogicalCPUs: runtime.NumCPU(), Capacity: float64(runtime.NumCPU()), CPUModel: "unknown", Affinity: "unknown", CPUQuota: "unknown", Hypervisor: "unknown", Raw: map[string]string{}}
	for _, p := range []string{"/proc/meminfo", "/proc/self/status", "/proc/self/cgroup", "/proc/self/mountinfo", "/sys/devices/system/node/online", "/sys/fs/cgroup/cpu.stat", "/proc/stat"} {
		s.Raw[p] = Read(p)
	}
	for _, line := range strings.Split(Read("/proc/cpuinfo"), "\n") {
		if strings.HasPrefix(line, "model name") {
			s.CPUModel = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			break
		}
	}
	for _, line := range strings.Split(s.Raw["/proc/self/status"], "\n") {
		if strings.HasPrefix(line, "Cpus_allowed_list:") {
			s.Affinity = strings.TrimSpace(strings.TrimPrefix(line, "Cpus_allowed_list:"))
			n := 0
			for _, r := range strings.Split(s.Affinity, ",") {
				v := strings.Split(r, "-")
				a, _ := strconv.Atoi(v[0])
				b := a
				if len(v) == 2 {
					b, _ = strconv.Atoi(v[1])
				}
				n += b - a + 1
			}
			if n > 0 {
				s.Capacity = float64(n)
			}
		}
	}
	for _, line := range strings.Split(s.Raw["/proc/meminfo"], "\n") {
		v := strings.Fields(line)
		if len(v) > 1 && v[0] == "MemAvailable:" {
			s.AvailableMemory, _ = strconv.ParseInt(v[1], 10, 64)
			s.AvailableMemory *= 1024
		}
	}
	cg := "/sys/fs/cgroup"
	for _, line := range strings.Split(Read("/proc/self/cgroup"), "\n") {
		if strings.HasPrefix(line, "0::") {
			cg = filepath.Join(cg, strings.TrimPrefix(line, "0::"))
		}
	}
	for p := cg; strings.HasPrefix(p, "/sys/fs/cgroup"); p = filepath.Dir(p) {
		q := Read(filepath.Join(p, "cpu.max"))
		v := strings.Fields(q)
		if len(v) == 2 && v[0] != "max" {
			a, _ := strconv.ParseFloat(v[0], 64)
			b, _ := strconv.ParseFloat(v[1], 64)
			if b > 0 && a/b < s.Capacity {
				s.Capacity = a / b
				s.CPUQuota = q
			}
		}
		m, e := strconv.ParseInt(Read(filepath.Join(p, "memory.max")), 10, 64)
		if e == nil && (s.MemoryLimit == 0 || m < s.MemoryLimit) {
			s.MemoryLimit = m
		}
		if p == "/sys/fs/cgroup" {
			break
		}
	}
	if s.MemoryLimit > 0 {
		u, _ := strconv.ParseInt(Read(filepath.Join(cg, "memory.current")), 10, 64)
		a := s.MemoryLimit - u
		if a < s.AvailableMemory {
			s.AvailableMemory = a
		}
	}
	for _, p := range []string{"/sys/class/dmi/id/product_name", "/sys/hypervisor/type"} {
		v := Read(p)
		if v != "unknown" {
			s.Raw[p] = v
		}
	}
	indexes, _ := filepath.Glob("/sys/devices/system/cpu/cpu0/cache/index*")
	for _, p := range indexes {
		for _, name := range []string{"level", "type", "size", "shared_cpu_list"} {
			s.Raw[filepath.Join(p, name)] = Read(filepath.Join(p, name))
		}
	}
	nodes, _ := filepath.Glob("/sys/devices/system/node/node[0-9]*")
	for _, p := range nodes {
		for _, name := range []string{"cpulist", "meminfo"} {
			s.Raw[filepath.Join(p, name)] = Read(filepath.Join(p, name))
		}
	}
	hyper := Read("/sys/hypervisor/type")
	if hyper != "unknown" {
		s.Hypervisor = hyper
	} else {
		dmi := Read("/sys/class/dmi/id/product_name")
		vendor := Read("/sys/class/dmi/id/sys_vendor")
		for _, known := range []string{"KVM", "QEMU", "VMware", "VirtualBox", "Xen"} {
			if strings.Contains(dmi, known) || strings.Contains(vendor, known) {
				s.Hypervisor = known
			}
		}
		if strings.Contains(vendor, "Microsoft") && strings.Contains(dmi, "Virtual Machine") {
			s.Hypervisor = "Hyper-V (DMI identification)"
		}
	}

	enrich(&s, "")
	return s
}
