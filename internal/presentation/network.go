package presentation

import (
	"encoding/json"
	"strconv"
	"vmbench/internal/benchmark/network"
	"vmbench/internal/report"
)

type transfer struct{ BPS, Seconds, Lost, Jitter *float64 }

func netTransfer(end map[string]json.RawMessage, key string) (transfer, bool) {
	var v struct {
		BPS     *float64 `json:"bits_per_second"`
		Seconds *float64 `json:"seconds"`
		Lost    *float64 `json:"lost_percent"`
		Jitter  *float64 `json:"jitter_ms"`
	}
	b, ok := end[key]
	if !ok || json.Unmarshal(b, &v) != nil {
		return transfer{}, false
	}
	return transfer{v.BPS, v.Seconds, v.Lost, v.Jitter}, true
}
func appendValue(dst []float64, p *float64, div float64) []float64 {
	if p != nil && finite(*p) && *p >= 0 {
		return append(dst, *p/div)
	}
	return dst
}
func networkModule(r report.Report, m Module) Module {
	if m.Command == "network server" {
		m.Metrics = []Metric{{"Listening endpoint", r.Settings.Bind + ":" + strconv.Itoa(r.Settings.Port)}}
		m.Assessment = "Server session ended; no throughput was measured by this command."
		return m
	}
	protocol := "TCP"
	if r.Settings.UDP {
		protocol = "UDP"
	}
	direction := "Client -> Server"
	if r.Settings.Reverse {
		direction = "Server -> Client"
	}
	if r.Settings.Bidir {
		direction = "Bidirectional"
	}
	runs, _ := decode[[]network.Result](r.Results["network test"])
	var speed, reverse, duration, reverseDuration, loss, jitter, reverseLoss, reverseJitter []float64
	t := Table{Title: "Receiver results per run", Headers: []string{text("count"), text("direction"), "Gbps", "s", "UDP loss %", "UDP jitter ms"}}
	for i, run := range runs {
		var end map[string]json.RawMessage
		if json.Unmarshal(run.End, &end) != nil || run.Error != "" {
			m.Status = "failed"
			m.Warnings = append(m.Warnings, "Network result is invalid; measurements are incomplete.")
			continue
		}
		v, ok := netTransfer(end, "sum_received")
		if !ok && r.Settings.UDP {
			v, ok = netTransfer(end, "sum")
		}
		if ok {
			speed = appendValue(speed, v.BPS, 1e9)
			duration = appendValue(duration, v.Seconds, 1)
			if r.Settings.UDP {
				loss = appendValue(loss, v.Lost, 1)
				jitter = appendValue(jitter, v.Jitter, 1)
			}
			rowDirection := direction
			if r.Settings.Bidir {
				rowDirection = "Client -> Server"
			}
			t.Rows = append(t.Rows, netRow(i+1, rowDirection, v))
		}
		if r.Settings.Bidir {
			v, ok = netTransfer(end, "sum_received_bidir_reverse")
			if ok {
				reverse = appendValue(reverse, v.BPS, 1e9)
				reverseDuration = appendValue(reverseDuration, v.Seconds, 1)
				if r.Settings.UDP {
					reverseLoss = appendValue(reverseLoss, v.Lost, 1)
					reverseJitter = appendValue(reverseJitter, v.Jitter, 1)
				}
				t.Rows = append(t.Rows, netRow(i+1, "Server -> Client", v))
			}
		}
	}
	m.Metrics = append(m.Metrics, Metric{"Protocol", protocol})
	if r.Settings.Bidir {
		m.Metrics = append(m.Metrics, Metric{protocol + " Client -> Server", medianText(speed, "Gbps", 2)}, Metric{protocol + " Server -> Client", medianText(reverse, "Gbps", 2)})
	} else {
		m.Metrics = append(m.Metrics, Metric{text("throughput"), medianText(speed, "Gbps", 2)})
	}
	durationValue := medianText(duration, "s", 2)
	if r.Settings.Bidir {
		durationValue = "Client -> Server " + durationValue + " / " + "Server -> Client " + medianText(reverseDuration, "s", 2)
	}
	if r.Settings.Bidir && r.Settings.UDP {
		direction += "; " + text("duration") + ": " + durationValue
	}
	m.Metrics = append(m.Metrics, Metric{text("direction"), direction})
	if !(r.Settings.Bidir && r.Settings.UDP) && len(duration) > 0 {
		m.Metrics = append(m.Metrics, Metric{text("duration"), durationValue})
	}
	if r.Settings.UDP {
		lossValue, jitterValue := rangeText(loss, "%", 2), rangeText(jitter, "ms", 3)
		if r.Settings.Bidir {
			lossValue = "Client -> Server " + lossValue + " / " + "Server -> Client " + rangeText(reverseLoss, "%", 2)
			jitterValue = "Client -> Server " + jitterValue + " / " + "Server -> Client " + rangeText(reverseJitter, "ms", 3)
		}
		if len(loss) > 0 || len(reverseLoss) > 0 {
			m.Metrics = append(m.Metrics, Metric{text("loss"), lossValue})
		}
		if len(jitter) > 0 || len(reverseJitter) > 0 {
			m.Metrics = append(m.Metrics, Metric{text("jitter"), jitterValue})
		}
	}
	m.Assessment = "The measured endpoint throughput is shown for this configuration."
	if len(speed) == 0 {
		m.Assessment = text("missing_note")
	}
	if r.Settings.Bidir && len(reverse) < len(speed) {
		m.Status = "failed"
		m.Warnings = append(m.Warnings, "Bidirectional measurements are incomplete; a receiver result is missing.")
	}
	m.Notes = []string{text("network_note")}
	columns := []int{0, 1, 2}
	if len(duration) > 0 || len(reverseDuration) > 0 {
		columns = append(columns, 3)
	}
	if r.Settings.UDP && len(loss)+len(reverseLoss) > 0 {
		columns = append(columns, 4)
	}
	if r.Settings.UDP && len(jitter)+len(reverseJitter) > 0 {
		columns = append(columns, 5)
	}
	filtered := Table{Title: t.Title}
	for _, column := range columns {
		filtered.Headers = append(filtered.Headers, t.Headers[column])
	}
	for _, row := range t.Rows {
		values := []string{}
		for _, column := range columns {
			values = append(values, row[column])
		}
		filtered.Rows = append(filtered.Rows, values)
	}
	m.Tables = append(m.Tables, filtered)
	for _, direction := range reportNetworkDirections(r) {
		for _, warning := range direction.observations.warnings {
			m.Warnings = append(m.Warnings, direction.name+": "+warning)
			if m.Status == "completed" {
				m.Status = "partial"
			}
		}
	}
	return m
}
func netRow(run int, direction string, v transfer) []string {
	values := []string{strconv.Itoa(run), direction}
	for _, x := range []struct {
		p   *float64
		div float64
	}{{v.BPS, 1e9}, {v.Seconds, 1}, {v.Lost, 1}, {v.Jitter, 1}} {
		s := "N/A"
		if x.p != nil && finite(*x.p) && *x.p >= 0 {
			s = number(*x.p/x.div, 3)
		}
		values = append(values, s)
	}
	return values
}
