package system

import (
	"strconv"
	"strings"
)

type CPUCounters struct {
	Total, Idle, Steal uint64
	Throttling         string
}

func Counters() CPUCounters {
	c := CPUCounters{Throttling: Throttling()}
	lines := strings.Split(Read("/proc/stat"), "\n")
	v := strings.Fields(lines[0])
	if len(v) < 9 {
		return c
	}
	for i := 1; i <= 8; i++ {
		n, _ := strconv.ParseUint(v[i], 10, 64)
		c.Total += n
		if i == 4 || i == 5 {
			c.Idle += n
		}
		if i == 8 {
			c.Steal = n
		}
	}
	return c
}
func Delta(a, b CPUCounters) map[string]any {
	r := map[string]any{"scope": "guest aggregate CPU counters over complete execution, including setup", "throttling_before": a.Throttling, "throttling_after": b.Throttling}
	if b.Total > a.Total && b.Idle >= a.Idle && b.Steal >= a.Steal {
		d := float64(b.Total - a.Total)
		r["busy_percent"] = 100 * (1 - float64(b.Idle-a.Idle)/d)
		r["steal_percent"] = 100 * float64(b.Steal-a.Steal) / d
	}
	return r
}
