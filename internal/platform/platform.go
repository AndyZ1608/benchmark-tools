// Package platform provides read-only runtime diagnostics without external build tools.
package platform

import (
	"bufio"
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ParseRelease never evaluates shell substitutions in OS metadata.
func ParseRelease(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "ID", "ID_LIKE", "VERSION_ID", "PRETTY_NAME":
		default:
			continue
		}
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			v = v[1 : len(v)-1]
		}
		out[k] = v
	}
	return out
}
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r > 126 {
			return ' '
		}
		return r
	}, s)
}

// Inspect checks ELF machine and interpreter without invoking ldd on a foreign binary.
func Inspect(path, arch string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() || st.Mode()&0111 == 0 {
		return "", fmt.Errorf("not an executable regular file: %s", path)
	}
	f, err := elf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	machine := elf.EM_X86_64
	if arch == "arm64" {
		machine = elf.EM_AARCH64
	} else if arch != "amd64" {
		return "", fmt.Errorf("unsupported architecture %s", arch)
	}
	if f.Machine != machine || f.Class != elf.ELFCLASS64 || f.Data != elf.ELFDATA2LSB {
		return "", fmt.Errorf("ELF architecture %s does not match linux/%s", f.Machine, arch)
	}
	linkage := "static"
	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP {
			b, e := io.ReadAll(io.LimitReader(p.Open(), 4096))
			if e != nil {
				return "", e
			}
			loader := strings.TrimRight(string(b), "\x00")
			if _, e = os.Stat(loader); e != nil {
				return "", fmt.Errorf("missing ELF loader %q: %w; use a matching libc target", loader, e)
			}
			linkage = "dynamic (" + loader + ")"
		}
	}
	return linkage, nil
}
func invoke(ctx context.Context, path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.WaitDelay = time.Second
	b, e := cmd.CombinedOutput()
	if e != nil {
		return "", fmt.Errorf("%s %s: %w: %s; check loader, shared libraries and CPU ISA", path, strings.Join(args, " "), e, safe(string(b)))
	}
	return string(b), nil
}

// Doctor performs version and capability queries only: no benchmarks, files, network or servers.
func Doctor(ctx context.Context, output io.Writer, root string) error {
	uringOK, uringReason := IOUring()
	aioOK, aioReason := LinuxAIO()
	return doctor(ctx, output, root, uringOK, uringReason, aioOK, aioReason)
}
func doctor(ctx context.Context, output io.Writer, root string, uringOK bool, uringReason string, aioOK bool, aioReason string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("runtime validation requires Linux")
	}
	if err := verifyManifest(root); err != nil {
		return err
	}
	b, _ := os.ReadFile("/etc/os-release")
	release := ParseRelease(string(b))
	kernel, _ := os.ReadFile("/proc/sys/kernel/osrelease")
	pretty := release["PRETTY_NAME"]
	if pretty == "" {
		pretty = release["ID"]
	}
	if pretty == "" {
		pretty = "unknown (os-release metadata unavailable)"
	}
	fmt.Fprintf(output, "OS: %s; linux/%s; kernel %s\n", safe(pretty), runtime.GOARCH, safe(strings.TrimSpace(string(kernel))))
	fmt.Fprintln(output, "Qualification: runtime diagnostics on this host; distribution certification is separate.")
	var failures []string
	for _, entry := range []struct{ name, prefix string }{{"compute-bench", "compute-bench-v1"}, {"fio", "fio-"}, {"stress-ng", "stress-ng"}, {"iperf3", "iperf 3."}} {
		path := filepath.Join(root, "engines", entry.name)
		linkage, err := Inspect(path, runtime.GOARCH)
		var version string
		if err == nil {
			version, err = invoke(ctx, path, "--version")
		}
		if err == nil && !strings.HasPrefix(strings.TrimSpace(version), entry.prefix) {
			err = fmt.Errorf("unexpected engine identity: %s", safe(version))
		}
		if err == nil && entry.name == "fio" {
			var help string
			help, err = invoke(ctx, path, "--enghelp")
			if err == nil {
				fmt.Fprintln(output, "VMBench Runtime Validation (no filesystem I/O)")
				for _, capability := range AsyncCapabilities(help, uringOK, uringReason, aioOK, aioReason) {
					fmt.Fprintf(output, "%s: %s - %s\n", capability.Name, capability.Status, safe(capability.Detail))
				}
				fmt.Fprintln(output, "[INFO] Disk validation deferred until an explicit target directory is selected; no synchronous fallback.")
			}
		}
		if err == nil && entry.name == "iperf3" {
			var help string
			help, err = invoke(ctx, path, "--help")
			if err == nil {
				for _, aliases := range [][]string{{"--json", "-J"}, {"--parallel", "-P"}, {"--omit", "-O"}} {
					if !HasOption(help, aliases...) {
						err = fmt.Errorf("iperf3 missing required option %s", aliases[0])
						break
					}
				}
			}
		}
		if err != nil {
			failures = append(failures, entry.name+": "+err.Error())
			fmt.Fprintf(output, "[ERROR] %s: FAILED - %s\n", entry.name, safe(err.Error()))
			continue
		}
		scan := bufio.NewScanner(strings.NewReader(version))
		scan.Scan()
		fmt.Fprintf(output, "[OK] %s: READY - %s; %s\n", entry.name, safe(scan.Text()), safe(linkage))
		if entry.name == "compute-bench" {
			fmt.Fprintln(output, "CPU engine: READY\nMemory engine: READY")
		}
	}
	fmt.Fprintf(output, "io_uring setup permitted: %t (%s).\n", uringOK, safe(uringReason))
	fmt.Fprintf(output, "Linux AIO permitted: %t (%s); per-filesystem direct I/O must still succeed.\n", aioOK, safe(aioReason))
	if len(failures) > 0 {
		return fmt.Errorf("runtime validation failed for %d engine(s); supply compatible engines or rerun initialization", len(failures))
	}
	return nil
}

func verifyManifest(root string) error {
	b, err := os.ReadFile(filepath.Join(root, "release-manifest.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var manifest struct {
		Schema int `json:"schema"`
		Files  map[string]struct {
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	if err = json.Unmarshal(b, &manifest); err != nil || manifest.Schema != 1 {
		return fmt.Errorf("invalid release-manifest.json")
	}
	for _, name := range []string{"vmbench", "engines/compute-bench", "engines/fio", "engines/stress-ng", "engines/iperf3"} {
		record, ok := manifest.Files[name]
		if !ok {
			return fmt.Errorf("release manifest missing %s", name)
		}
		file, err := os.Open(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, err = io.Copy(hash, file)
		file.Close()
		if err != nil {
			return err
		}
		if hex.EncodeToString(hash.Sum(nil)) != record.SHA256 {
			return fmt.Errorf("release checksum mismatch: %s; restore the verified archive", name)
		}
	}
	return nil
}

type Capability struct{ Name, Status, Detail string }

const (
	Ready                 = "READY"
	AvailableNotValidated = "AVAILABLE_NOT_VALIDATED"
	Unavailable           = "UNAVAILABLE"
	Failed                = "FAILED"
)

// AsyncCapabilities distinguishes compiled names and setup permission from real
// direct I/O. Only a target-specific completed-I/O test can establish readiness.
func AsyncCapabilities(compiled string, uringOK bool, uringReason string, aioOK bool, aioReason string) []Capability {
	out := []Capability{}
	for _, probe := range []struct {
		name      string
		permitted bool
		reason    string
	}{{"io_uring", uringOK, uringReason}, {"libaio", aioOK, aioReason}} {
		c := Capability{Name: probe.name, Status: Unavailable, Detail: "not advertised by fio --enghelp"}
		for _, name := range strings.Fields(compiled) {
			if name != probe.name {
				continue
			}
			c.Detail = probe.reason
			if probe.permitted {
				c.Status = AvailableNotValidated
				c.Detail += "; requires filesystem-specific direct I/O and achieved-depth validation"
			}
			break
		}
		out = append(out, c)
	}
	return out
}

// HasOption accepts documented short/long aliases, without substring matches.
func HasOption(help string, aliases ...string) bool {
	for _, field := range strings.Fields(help) {
		for _, token := range strings.Split(field, ",") {
			for _, alias := range aliases {
				if token == alias {
					return true
				}
			}
		}
	}
	return false
}

// DescribeUringPolicy reports policy only. A zero sysctl does not override
// seccomp, SELinux, capabilities, kernel support, or the setup syscall result.
func DescribeUringPolicy(disabled, group string) string {
	switch disabled {
	case "0":
		return "io_uring_disabled=0 (sysctl permits setup; other runtime restrictions may apply)"
	case "1":
		if group == "-1" || group == "4294967295" {
			return "io_uring_disabled=1 (requires CAP_SYS_ADMIN; io_uring_group=-1 grants no group exemption; actual setup permission is probed)"
		}
		if group == "" {
			group = "unavailable"
		}
		return fmt.Sprintf("io_uring_disabled=1 (requires CAP_SYS_ADMIN or membership in io_uring_group=%s; actual setup permission is probed)", group)
	case "2":
		return "io_uring_disabled=2 (setup disabled for all processes)"
	default:
		return "unrecognized io_uring_disabled=" + disabled + "; actual setup permission is probed"
	}
}
