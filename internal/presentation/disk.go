package presentation

import (
	"fmt"
	"strconv"
	"vmbench/internal/benchmark/disk"
	"vmbench/internal/report"
	"vmbench/internal/statistics"
)

type diskGroup struct {
	Depth                                int
	ReadIOPS, WriteIOPS, ReadBW, WriteBW []float64
	ReadP99, WriteP99                    []float64
	ReadMean, WriteMean                  []float64
}

func percentile(l disk.Latency, p float64) (float64, bool) {
	for k, v := range l.Percentile {
		x, e := strconv.ParseFloat(k, 64)
		if e == nil && x == p && finite(v) && v >= 0 {
			return v / 1e6, true
		}
	}
	return 0, false
}
func diskGroups(r report.Report) ([]diskGroup, []disk.Result) {
	runs, ok := decode[[]disk.Result](r.Results["disk test"])
	if !ok {
		return nil, nil
	}
	depths := []int{r.Settings.Depth}
	if r.Settings.Profile == "latency" {
		depths = []int{1}
	}
	if r.Settings.Profile == "qd-scaling" {
		depths = []int{1, 4, 16, 32, 64}
	}
	reps := r.Settings.Repetitions
	if reps < 1 {
		return nil, runs
	}
	groups := []diskGroup{}
	for i, run := range runs {
		groupIndex := i / reps
		if groupIndex >= len(depths) {
			break
		}
		if len(groups) <= groupIndex {
			groups = append(groups, diskGroup{Depth: depths[groupIndex]})
		}
		g := &groups[groupIndex]
		readIOPS, writeIOPS, readBW, writeBW := 0.0, 0.0, 0.0, 0.0
		readP99, writeP99 := []float64{}, []float64{}
		readMean, writeMean := []float64{}, []float64{}
		valid := len(run.Jobs) > 0
		for _, j := range run.Jobs {
			if j.Error != 0 {
				valid = false
			}
			if !finite(j.Read.IOPS) || !finite(j.Write.IOPS) || !finite(j.Read.BWBytes) || !finite(j.Write.BWBytes) || j.Read.IOPS < 0 || j.Write.IOPS < 0 || j.Read.BWBytes < 0 || j.Write.BWBytes < 0 {
				valid = false
			}
			readIOPS += j.Read.IOPS
			writeIOPS += j.Write.IOPS
			readBW += j.Read.BWBytes
			writeBW += j.Write.BWBytes
			if v, ok := meanLatency(j.Read); ok {
				readMean = append(readMean, v)
			}
			if v, ok := meanLatency(j.Write); ok {
				writeMean = append(writeMean, v)
			}
			if v, ok := percentile(j.Read.Completion, 99); ok && j.Read.IOPS > 0 {
				readP99 = append(readP99, v)
			}
			if v, ok := percentile(j.Write.Completion, 99); ok && j.Write.IOPS > 0 {
				writeP99 = append(writeP99, v)
			}
		}
		if valid {
			g.ReadMean = append(g.ReadMean, readMean...)
			g.WriteMean = append(g.WriteMean, writeMean...)
			g.ReadP99 = append(g.ReadP99, readP99...)
			g.WriteP99 = append(g.WriteP99, writeP99...)
			g.ReadIOPS = append(g.ReadIOPS, readIOPS)
			g.WriteIOPS = append(g.WriteIOPS, writeIOPS)
			g.ReadBW = append(g.ReadBW, readBW/(1<<20))
			g.WriteBW = append(g.WriteBW, writeBW/(1<<20))
		}
	}
	return groups, runs
}
func description(v []float64) statistics.Summary { s, _ := statistics.Describe(v); return s }
func medianText(v []float64, u string, d int) string {
	if len(v) == 0 {
		return "N/A"
	}
	return unit(description(v).Median, u, d)
}
func diskProfile(p string) string {
	switch p {
	case "randread-4k":
		return "Random Read 4K"
	case "randwrite-4k":
		return "Random Write 4K"
	case "mixed-70-30":
		return "Random Read/Write 70/30"
	case "seqread-1m":
		return "Sequential Read 1M"
	case "seqwrite-1m":
		return "Sequential Write 1M"
	case "latency":
		return "Low queue-depth read latency"
	case "qd-scaling":
		return "Queue-depth scaling (random read 4K)"
	}
	return p
}
func diskModule(r report.Report, m Module, threshold float64) Module {
	groups, runs := diskGroups(r)
	for _, run := range runs {
		if len(run.Jobs) == 0 {
			m.Status = "failed"
			m.Warnings = append(m.Warnings, "fio returned no jobs; measurements are invalid.")
		}
		for _, job := range run.Jobs {
			if !finite(job.Read.IOPS) || !finite(job.Write.IOPS) || !finite(job.Read.BWBytes) || !finite(job.Write.BWBytes) || job.Read.IOPS < 0 || job.Write.IOPS < 0 || job.Read.BWBytes < 0 || job.Write.BWBytes < 0 {
				m.Status = "failed"
				m.Warnings = append(m.Warnings, "fio returned invalid throughput values; measurements cannot be used.")
			}
			if job.Error != 0 {
				m.Status = "failed"
				m.Warnings = append(m.Warnings, "fio reported an I/O error; inspect the error and disk target.")
			}
		}
	}
	m.Metrics = []Metric{{text("workload"), diskProfile(r.Settings.Profile)}}
	m.Notes = []string{text("disk_note"), text("percentile_note"), text("baseline")}
	if len(groups) == 0 {
		m.Assessment = text("missing_note")
		return m
	}
	mixed := r.Settings.Profile == "mixed-70-30"
	write := r.Settings.Profile == "randwrite-4k" || r.Settings.Profile == "seqwrite-1m"
	var stats []statistics.Summary
	if r.Settings.Profile == "qd-scaling" {
		t := Table{Title: text("workload"), Headers: []string{"QD", text("iops"), text("throughput"), text("p99"), text("cv")}}
		for _, g := range groups {
			stats = append(stats, description(g.ReadIOPS))
			t.Rows = append(t.Rows, []string{strconv.Itoa(g.Depth), medianText(g.ReadIOPS, "IOPS", 0), medianText(g.ReadBW, "MiB/s", 2), rangeText(g.ReadP99, "ms", 3), cvText(description(g.ReadIOPS))})
		}
		first, last := groups[0], groups[len(groups)-1]
		m.Metrics = append(m.Metrics, Metric{"QD" + strconv.Itoa(first.Depth), medianText(first.ReadIOPS, "IOPS", 0) + "; mean " + rangeText(first.ReadMean, "ms", 3) + "; p99 " + rangeText(first.ReadP99, "ms", 3)})
		if first.Depth != last.Depth {
			m.Metrics = append(m.Metrics, Metric{"QD" + strconv.Itoa(last.Depth), medianText(last.ReadIOPS, "IOPS", 0) + "; mean " + rangeText(last.ReadMean, "ms", 3) + "; p99 " + rangeText(last.ReadP99, "ms", 3)})
			base := description(first.ReadBW).Median
			if base > 0 && len(last.ReadBW) > 0 {
				m.Metrics = append(m.Metrics, Metric{"Throughput scaling", number(description(last.ReadBW).Median/base, 2) + "x"})
			}
			if len(first.ReadP99) > 0 && len(last.ReadP99) > 0 {
				m.Notes = append(m.Notes, "Compare the p99 ranges above to observe the latency change; these are separate job/run distributions.")
				maxFirst, maxLast := first.ReadP99[0], last.ReadP99[0]
				for _, v := range first.ReadP99 {
					if v > maxFirst {
						maxFirst = v
					}
				}
				for _, v := range last.ReadP99 {
					if v > maxLast {
						maxLast = v
					}
				}
				if maxLast > maxFirst {
					notice := "Highest observed job/run p99 increased: " + unit(maxFirst, "ms", 3) + " -> " + unit(maxLast, "ms", 3)
					m.Notes = append(m.Notes, notice)
					if number(maxFirst, 3) != number(maxLast, 3) {
						m.Warnings = append(m.Warnings, notice)
					}
				}
			}
		}
		m.Tables = append(m.Tables, t)
	} else {
		g := groups[0]
		m.Metrics[0].Value += fmt.Sprintf("; QD%d x %d %s", g.Depth, r.Settings.Jobs, "jobs")
		if mixed {
			stats = []statistics.Summary{description(g.ReadIOPS), description(g.WriteIOPS)}
			m.Metrics = append(m.Metrics, Metric{text("iops"), text("read") + " " + medianText(g.ReadIOPS, "IOPS", 0) + " / " + text("write") + " " + medianText(g.WriteIOPS, "IOPS", 0)}, Metric{text("throughput"), text("read") + " " + medianText(g.ReadBW, "MiB/s", 2) + " / " + text("write") + " " + medianText(g.WriteBW, "MiB/s", 2)}, Metric{text("average"), text("read") + " " + rangeText(g.ReadMean, "ms", 3) + " / " + text("write") + " " + rangeText(g.WriteMean, "ms", 3)}, Metric{text("p99"), text("read") + " " + rangeText(g.ReadP99, "ms", 3) + " / " + text("write") + " " + rangeText(g.WriteP99, "ms", 3)})
		} else {
			values, bw, p99, mean := g.ReadIOPS, g.ReadBW, g.ReadP99, g.ReadMean
			if write {
				values, bw, p99, mean = g.WriteIOPS, g.WriteBW, g.WriteP99, g.WriteMean
			}
			stats = []statistics.Summary{description(values)}
			m.Metrics = append(m.Metrics, Metric{text("iops"), medianText(values, "IOPS", 0)}, Metric{text("throughput"), medianText(bw, "MiB/s", 2)}, Metric{text("average"), rangeText(mean, "ms", 3)}, Metric{text("p99"), rangeText(p99, "ms", 3)})
		}
	}
	label, assessment := stability(stats, threshold)
	m.Metrics = append(m.Metrics, Metric{text("stability"), label})
	m.Assessment = assessment
	if r.Settings.Details {
		t := Table{Title: "fio results (separate distributions)", Headers: []string{text("count"), "Job", text("direction"), "IOPS", "MiB/s", "Average ms", "p95 ms", "p99 ms", "p99.9 ms", text("elapsed")}}
		for i, run := range runs {
			for j, job := range run.Jobs {
				for _, d := range []struct {
					name string
					v    disk.Direction
				}{{"read", job.Read}, {"write", job.Write}} {
					if d.v.IOPS <= 0 && d.v.BWBytes <= 0 {
						continue
					}
					ps := []string{}
					for _, p := range []float64{95, 99, 99.9} {
						v, ok := percentile(d.v.Completion, p)
						value := "N/A"
						if ok {
							value = number(v, 3)
						}
						ps = append(ps, value)
					}
					mean := "N/A"
					if value, ok := meanLatency(d.v); ok {
						mean = number(value, 3)
					}
					t.Rows = append(t.Rows, []string{strconv.Itoa(i + 1), strconv.Itoa(j + 1), text(d.name), number(d.v.IOPS, 0), number(d.v.BWBytes/(1<<20), 2), mean, ps[0], ps[1], ps[2], unit(d.v.Runtime/1000, "s", 2)})
				}
			}
		}
		m.Tables = append(m.Tables, t)
	}
	return m
}

// A range preserves individual job/run means; no unweighted pooled mean is invented.
// Missing clat_ns.mean decodes to zero in the existing parser; show N/A for it.
func meanLatency(direction disk.Direction) (float64, bool) {
	value := direction.Completion.Mean
	return value / 1e6, direction.IOPS > 0 && value > 0 && finite(value)
}
