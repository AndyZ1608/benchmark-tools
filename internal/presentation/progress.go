package presentation

import (
	"fmt"
	"io"
	"strings"
	"time"
	"vmbench/internal/runner"
)

// AutomaticProgress keeps search and capability telemetry in --details while
// preserving unknown notices. Terminal workload failures are also in Result.
func AutomaticProgress(w io.Writer, details bool) func(string) {
	return func(line string) {
		if !details {
			switch line {
			case "[OK] System detected":
				line = "Benchmark running..."
			case "[DISK] Prepared dataset; finding observed IOPS and sequential throughput":
				line = "Disk: Measuring..."
			case "[OK] Temporary files removed":
				return
			}
			if strings.HasPrefix(line, "[DISK] Preparing ") {
				line = "Disk: Preparing temporary files..."
			}
			for _, engine := range []string{"io_uring", "libaio"} {
				if strings.HasPrefix(line, "[OK] "+engine+": READY - ") || strings.HasPrefix(line, "[INFO] "+engine+": FAILED on selected target: ") {
					return
				}
			}
			for _, workload := range []string{"integer-xorshift64", "fp-multiply-add", "triad", "randread-4k", "seqread-1m", "randwrite-4k", "seqwrite-1m", "mixed-70-30", "TCP TX", "TCP RX"} {
				if strings.HasPrefix(line, workload+": ") && (strings.HasSuffix(line, " completed") || strings.HasSuffix(line, ": best configuration confirmed")) {
					return
				}
			}
		}
		fmt.Fprintln(w, SafeText(line))
	}
}

// Progress changes only at module boundaries, outside engine measurement timers.
// No invented workload percentages or background polling are used.
type Progress struct {
	Writer io.Writer

	Interactive bool
	started     time.Time
	command     string
	width       int
}

func (p *Progress) Start(command string) {
	p.command = command
	p.started = time.Now()
	prefix := ""
	if p.Interactive {
		prefix = "\r"
	}
	line := command + ": " + text("preparing") + "..."
	p.width = len([]rune(line))
	fmt.Fprint(p.Writer, prefix+line)
	if !p.Interactive {
		fmt.Fprintln(p.Writer)
	}
}
func (p *Progress) Finish(state string) {
	prefix := ""
	if p.Interactive {
		prefix = "\r"
	}
	line := p.command + ": " + text(state) + " (" + unit(time.Since(p.started).Seconds(), "s", 1) + ")"
	if p.Interactive && p.width > len([]rune(line)) {
		line += strings.Repeat(" ", p.width-len([]rune(line)))
	}
	fmt.Fprintln(p.Writer, prefix+line)
}

// Observe announces real external-engine starts; it never estimates elapsed-time percentages.
func (p *Progress) Observe(event runner.Event) {
	if event.Phase != "" {
		if p.Interactive {
			fmt.Fprint(p.Writer, "\r"+strings.Repeat(" ", p.width)+"\r")
			p.width = 0
		}
		fmt.Fprintln(p.Writer, clean(event.Message))
		return
	}
	if !event.Starting || len(event.Args) == 0 || event.Args[0] == "--version" {
		return
	}
	phase := event.Name
	switch event.Name {
	case "compute-bench":
		if len(event.Args) > 1 {
			phase = event.Args[0] + "; " + event.Args[1] + " " + text("workers")
		}
	case "fio":
		phase = "Disk dataset preparation"
		for _, arg := range event.Args {
			if arg == "--time_based=1" {
				phase = "Disk measurement"
			}
		}
	case "stress-ng":
		phase = "CPU stress workload"
	case "iperf3":
		phase = "Network engine"
	}
	line := p.command + ": " + "Launching " + phase
	if p.Interactive {
		width := len([]rune(line))
		padding := ""
		if p.width > width {
			padding = strings.Repeat(" ", p.width-width)
		}
		fmt.Fprint(p.Writer, "\r"+line+padding)
		p.width = width
	} else {
		fmt.Fprintln(p.Writer, line)
	}
}
