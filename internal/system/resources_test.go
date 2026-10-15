package system

import (
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range files {
		p := filepath.Join(root, name)
		if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(p, []byte(data), 0600); e != nil {
			t.Fatal(e)
		}
	}
	return root
}
func TestAncestorLimitsUseAncestorUsage(t *testing.T) {
	root := fixture(t, map[string]string{
		"/proc/meminfo":                      "MemTotal: 8388608 kB\nMemAvailable: 6291456 kB\n",
		"/proc/self/cgroup":                  "0::/tenant/leaf\n",
		"/proc/self/mountinfo":               "1 0 0:1 / /sys/fs/cgroup rw - cgroup2 cgroup rw\n",
		"/sys/fs/cgroup/tenant/leaf/cpu.max": "max 100000", "/sys/fs/cgroup/tenant/cpu.max": "800000 100000",
		"/sys/fs/cgroup/tenant/leaf/cpuset.cpus.effective": "0-11", "/sys/fs/cgroup/tenant/memory.max": "4294967296", "/sys/fs/cgroup/tenant/memory.current": "3221225472",
		"/sys/fs/cgroup/tenant/leaf/memory.max": "max", "/sys/fs/cgroup/tenant/leaf/memory.current": "1000",
		"/proc/cpuinfo": "flags : sse2 avx2\n", "/proc/vmstat": "pswpin 2\npswpout 3\n",
		"/sys/devices/system/cpu/cpu0/cache/index3/level": "3", "/sys/devices/system/cpu/cpu0/cache/index3/type": "Unified", "/sys/devices/system/cpu/cpu0/cache/index3/size": "8M", "/sys/devices/system/cpu/cpu0/cache/index3/shared_cpu_list": "0-1",
		"/sys/devices/system/cpu/cpu1/cache/index3/level": "3", "/sys/devices/system/cpu/cpu1/cache/index3/type": "Unified", "/sys/devices/system/cpu/cpu1/cache/index3/size": "8M", "/sys/devices/system/cpu/cpu1/cache/index3/shared_cpu_list": "0-1",
		"/sys/devices/system/node/node0/cpulist": "0-7", "/sys/devices/system/node/node1/cpulist": "8-15", "/sys/class/net/eth0/operstate": "up", "/sys/class/net/eth0/speed": "10000",
	})
	s := Info{LogicalCPUs: 16, Affinity: "0-15", Raw: map[string]string{}}
	enrich(&s, root)
	if s.Capacity != 8 || s.AvailableMemory != 1<<30 || s.TotalMemory != 8<<30 || s.MemoryLimit != 4<<30 || s.LastLevelCache != 8<<20 || s.NUMANodes != 2 || s.SwapIn != 2 || len(s.Interfaces) != 1 || s.Interfaces[0].SpeedMbps != 10000 {
		t.Fatalf("unexpected constrained resources: %+v", s)
	}
}
func TestCgroupMountRootAndV1Restrictions(t *testing.T) {
	root := fixture(t, map[string]string{"/proc/meminfo": "MemAvailable: 2097152 kB", "/proc/self/cgroup": "2:cpu,cpuacct:/tenant/job\n3:memory:/tenant/job", "/proc/self/mountinfo": "1 0 0:1 /tenant /cg/cpu rw - cgroup cgroup rw,cpu,cpuacct\n2 0 0:2 /tenant /cg/memory rw - cgroup cgroup rw,memory", "/cg/cpu/job/cpu.cfs_quota_us": "150000", "/cg/cpu/job/cpu.cfs_period_us": "100000", "/cg/memory/job/memory.limit_in_bytes": "1073741824", "/cg/memory/job/memory.usage_in_bytes": "536870912"})
	s := Info{LogicalCPUs: 8, Affinity: "0-3", Raw: map[string]string{}}
	enrich(&s, root)
	if s.Capacity != 1.5 || s.AvailableMemory != 512<<20 {
		t.Fatal(s)
	}
}
func TestUnknownRestrictedMemoryUsageDisablesAllocation(t *testing.T) {
	root := fixture(t, map[string]string{"/proc/meminfo": "MemAvailable: 2097152 kB", "/proc/self/cgroup": "0::/", "/proc/self/mountinfo": "1 0 0:1 / /sys/fs/cgroup rw - cgroup2 cgroup rw", "/sys/fs/cgroup/memory.max": "1073741824"})
	s := Info{LogicalCPUs: 8, Affinity: "0-1,4", Raw: map[string]string{}}
	enrich(&s, root)
	if s.Capacity != 3 || s.AvailableMemory != 0 || len(s.DetectionWarnings) == 0 {
		t.Fatal(s)
	}
}
func TestZeroMemoryHighPreventsAutomaticAllocation(t *testing.T) {
	root := fixture(t, map[string]string{"/proc/meminfo": "MemAvailable: 2097152 kB", "/proc/self/cgroup": "0::/", "/proc/self/mountinfo": "1 0 0:1 / /sys/fs/cgroup rw - cgroup2 cgroup rw", "/sys/fs/cgroup/memory.high": "0", "/sys/fs/cgroup/memory.current": "4096"})
	s := Info{LogicalCPUs: 2, Affinity: "0-1", Raw: map[string]string{}}
	enrich(&s, root)
	if s.AvailableMemory != 0 {
		t.Fatal(s)
	}
}
func TestNominalCPUCountAndEqualQuotaRemainDistinct(t *testing.T) {
	root := fixture(t, map[string]string{"/sys/devices/system/cpu/online": "0-7", "/proc/self/cgroup": "0::/", "/proc/self/mountinfo": "1 0 0:1 / /sys/fs/cgroup rw - cgroup2 cgroup rw", "/sys/fs/cgroup/cpu.max": "200000 100000"})
	s := Info{LogicalCPUs: 2, Affinity: "0-1", Raw: map[string]string{}}
	enrich(&s, root)
	if s.LogicalCPUs != 8 || s.Capacity != 2 || s.CPUQuota != "200000 100000" {
		t.Fatal(s)
	}
}
