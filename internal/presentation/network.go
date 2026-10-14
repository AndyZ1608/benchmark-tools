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
func networkModule(r report.Report, m Module, lang string) Module {
	if m.Command == "network server" {
		m.Metrics = []Metric{{tr(lang, "Listening endpoint", "Địa chỉ lắng nghe"), r.Settings.Bind + ":" + strconv.Itoa(r.Settings.Port)}}
		m.Assessment = tr(lang, "Server session ended; no throughput was measured by this command.", "Phiên máy chủ đã kết thúc; lệnh này không đo tốc độ truyền.")
		return m
	}
	protocol := "TCP"
	if r.Settings.UDP {
		protocol = "UDP"
	}
	direction := tr(lang, "Client -> Server", "Máy khách -> Máy chủ")
	if r.Settings.Reverse {
		direction = tr(lang, "Server -> Client", "Máy chủ -> Máy khách")
	}
	if r.Settings.Bidir {
		direction = tr(lang, "Bidirectional", "Hai chiều")
	}
	runs, _ := decode[[]network.Result](r.Results["network test"])
	var speed, reverse, duration, reverseDuration, loss, jitter, reverseLoss, reverseJitter []float64
	t := Table{Title: tr(lang, "Receiver results per run", "Kết quả đầu nhận theo lần đo"), Headers: []string{text(lang, "count"), text(lang, "direction"), "Gbps", "s", "UDP loss %", "UDP jitter ms"}}
	for i, run := range runs {
		var end map[string]json.RawMessage
		if json.Unmarshal(run.End, &end) != nil || run.Error != "" {
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
				rowDirection = tr(lang, "Client -> Server", "Máy khách -> Máy chủ")
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
				t.Rows = append(t.Rows, netRow(i+1, tr(lang, "Server -> Client", "Máy chủ -> Máy khách"), v))
			}
		}
	}
	if r.Settings.Bidir {
		m.Metrics = append(m.Metrics, Metric{protocol + " " + tr(lang, "Client -> Server", "Máy khách -> Máy chủ"), medianText(speed, "Gbps", 2)}, Metric{protocol + " " + tr(lang, "Server -> Client", "Máy chủ -> Máy khách"), medianText(reverse, "Gbps", 2)})
	} else {
		m.Metrics = append(m.Metrics, Metric{protocol + " " + text(lang, "throughput"), medianText(speed, "Gbps", 2)})
	}
	durationValue := medianText(duration, "s", 2)
	if r.Settings.Bidir {
		durationValue = tr(lang, "Client -> Server ", "Máy khách -> Máy chủ ") + durationValue + " / " + tr(lang, "Server -> Client ", "Máy chủ -> Máy khách ") + medianText(reverseDuration, "s", 2)
	}
	if r.Settings.Bidir && r.Settings.UDP {
		direction += "; " + text(lang, "duration") + ": " + durationValue
	}
	m.Metrics = append(m.Metrics, Metric{text(lang, "direction"), direction})
	if !(r.Settings.Bidir && r.Settings.UDP) {
		m.Metrics = append(m.Metrics, Metric{text(lang, "duration"), durationValue})
	}
	if r.Settings.UDP {
		lossValue, jitterValue := rangeText(loss, "%", 2), rangeText(jitter, "ms", 3)
		if r.Settings.Bidir {
			lossValue = tr(lang, "Client -> Server ", "Máy khách -> Máy chủ ") + lossValue + " / " + tr(lang, "Server -> Client ", "Máy chủ -> Máy khách ") + rangeText(reverseLoss, "%", 2)
			jitterValue = tr(lang, "Client -> Server ", "Máy khách -> Máy chủ ") + jitterValue + " / " + tr(lang, "Server -> Client ", "Máy chủ -> Máy khách ") + rangeText(reverseJitter, "ms", 3)
		}
		m.Metrics = append(m.Metrics, Metric{text(lang, "loss"), lossValue}, Metric{text(lang, "jitter"), jitterValue})
	}
	m.Assessment = tr(lang, "The measured endpoint throughput is shown for this configuration.", "Tốc độ truyền giữa hai đầu cuối được ghi nhận theo cấu hình này.")
	if len(speed) == 0 {
		m.Assessment = text(lang, "missing_note")
	}
	m.Notes = []string{text(lang, "network_note")}
	m.Tables = append(m.Tables, t, Table{Title: tr(lang, "RTT", "RTT"), Headers: []string{text(lang, "status")}, Rows: [][]string{{text(lang, "rtt_note")}}})
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
