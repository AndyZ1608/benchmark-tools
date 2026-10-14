package presentation

import (
	"fmt"
	"io"
	"strings"
	"time"
	"vmbench/internal/runner"
)

// Progress changes only at module boundaries, outside engine measurement timers.
// No invented workload percentages or background polling are used.
type Progress struct {
	Writer      io.Writer
	Lang        string
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
	line := command + ": " + text(p.Lang, "preparing") + "..."
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
	line := p.command + ": " + text(p.Lang, state) + " (" + unit(time.Since(p.started).Seconds(), "s", 1) + ")"
	if p.Interactive && p.width > len([]rune(line)) {
		line += strings.Repeat(" ", p.width-len([]rune(line)))
	}
	fmt.Fprintln(p.Writer, prefix+line)
}

// Observe announces real external-engine starts; it never estimates elapsed-time percentages.
func (p *Progress) Observe(event runner.Event) {
	if !event.Starting || len(event.Args) == 0 || event.Args[0] == "--version" {
		return
	}
	phase := event.Name
	switch event.Name {
	case "compute-bench":
		if len(event.Args) > 1 {
			phase = event.Args[0] + "; " + event.Args[1] + " " + text(p.Lang, "workers")
		}
	case "fio":
		phase = tr(p.Lang, "Disk dataset preparation", "Chuẩn bị dữ liệu đĩa")
		for _, arg := range event.Args {
			if arg == "--time_based=1" {
				phase = tr(p.Lang, "Disk measurement", "Đo đĩa")
			}
		}
	case "stress-ng":
		phase = tr(p.Lang, "CPU stress workload", "Tác vụ stress CPU")
	case "iperf3":
		phase = tr(p.Lang, "Network engine", "Tác vụ mạng")
	}
	line := p.command + ": " + tr(p.Lang, "Launching ", "Khởi chạy ") + phase
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
