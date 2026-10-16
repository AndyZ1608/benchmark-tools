package presentation

import (
	"encoding/json"
	"fmt"

	automatic "vmbench/internal/benchmark/auto"
	"vmbench/internal/benchmark/network"
)

type networkObservations struct {
	speed, seconds, loss, jitter []float64
	retransmits                  []uint64
	warnings                     []string
}

// Receiver throughput and sender retransmit counters describe different
// observations. In particular, TCP retransmissions never imply UDP packet loss.
func (o *networkObservations) add(raw network.Result, receiver, sender string, udp bool) {
	var end map[string]json.RawMessage
	if raw.Error != "" || json.Unmarshal(raw.End, &end) != nil {
		o.warnings = append(o.warnings, "Invalid network result; transfer measurements are incomplete")
		if raw.Error != "" {
			o.warnings = append(o.warnings, raw.Error)
		}
		return
	}
	value, ok := netTransfer(end, receiver)
	if !ok && udp && receiver == "sum_received" {
		value, ok = netTransfer(end, "sum")
	}
	if !ok || value.BPS == nil || !finite(*value.BPS) || *value.BPS < 0 || value.Seconds == nil || !finite(*value.Seconds) || *value.Seconds <= 0 {
		o.warnings = append(o.warnings, "Receiver throughput or duration is missing/invalid; transfer measurements are incomplete")
		return
	}
	o.speed = append(o.speed, *value.BPS)
	o.seconds = append(o.seconds, *value.Seconds)
	if udp {
		if value.Lost != nil {
			if finite(*value.Lost) && *value.Lost >= 0 && *value.Lost <= 100 {
				o.loss = append(o.loss, *value.Lost)
			} else {
				o.warnings = append(o.warnings, "Invalid UDP packet-loss evidence")
			}
		}
		if value.Jitter != nil {
			if finite(*value.Jitter) && *value.Jitter >= 0 {
				o.jitter = append(o.jitter, *value.Jitter)
			} else {
				o.warnings = append(o.warnings, "Invalid UDP jitter evidence")
			}
		}
		return
	}
	if bytes, present := end[sender]; present {
		var sent struct {
			Retransmits *uint64 `json:"retransmits"`
		}
		if json.Unmarshal(bytes, &sent) != nil {
			o.warnings = append(o.warnings, "Invalid TCP retransmit counter; retransmission telemetry is incomplete")
		} else if sent.Retransmits != nil {
			o.retransmits = append(o.retransmits, *sent.Retransmits)
		}
	}
}

func automaticNetworkObservations(peak automatic.Peak) networkObservations {
	var observations networkObservations
	if peak.Best != nil {
		for _, sample := range peak.Best.Samples {
			if raw, ok := decode[network.Result](sample.Raw); ok {
				observations.add(raw, "sum_received", "sum_sent", false)
			}
		}
	}
	return observations
}

func networkRate(bitsPerSecond float64) string {
	if bitsPerSecond >= 1e9 {
		return unit(bitsPerSecond/1e9, "Gbps", 2)
	}
	return unit(bitsPerSecond/1e6, "Mbps", 2)
}

// Never sum independent runs or round counts into fractional retransmissions.
func countRange(values []uint64) string {
	if len(values) == 0 {
		return ""
	}
	lo, hi := values[0], values[0]
	for _, value := range values[1:] {
		lo, hi = min(lo, value), max(hi, value)
	}
	value := groupedCount(lo)
	if lo != hi {
		value += " - " + groupedCount(hi)
	}
	if len(values) > 1 {
		value += " (per-run range)"
	}
	return value
}

func groupedCount(value uint64) string {
	s := fmt.Sprint(value)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
