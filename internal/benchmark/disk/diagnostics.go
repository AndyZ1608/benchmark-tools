package disk

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"vmbench/internal/config"
)

func measurementFailure(c config.Config, depth, repetition int, r Result, raw []byte, cause error) error {
	var lines []string
	lines = append(lines, fmt.Sprintf("Workload        : %s", c.Profile), fmt.Sprintf("I/O Engine      : %s", c.Engine), fmt.Sprintf("Requested QD    : %d", depth))
	jobs := r.Jobs
	if len(jobs) == 0 {
		jobs = []Job{{}}
	}
	rw, _, _ := profile(c)
	for i, job := range jobs {
		name := jobName(job, i)
		if len(r.Jobs) == 0 {
			name = "unavailable"
		}
		lines = append(lines, "Job             : "+name)
		for _, direction := range activeDirections(rw) {
			v, label := job.Read, "Read Bytes"
			if direction == "write" {
				v, label = job.Write, "Write Bytes"
			}
			value := func(key string, n uint64) string {
				if v.present == nil || fieldMissing(v.present, key) {
					return "unavailable"
				}
				return fmt.Sprint(n)
			}
			difference := "unavailable"
			if v.present != nil && !fieldMissing(v.present, "io_bytes") && !fieldMissing(v.present, "total_ios") {
				difference = counterObservation(jobName(job, i), direction, v, workloadBlock(c)).ByteDifference
			}
			lines = append(lines, fmt.Sprintf("%-16s: %s", label, value("io_bytes", v.IOBytes)), "Total I/Os      : "+value("total_ios", v.TotalIOs), "Byte Difference : "+difference, "Short I/Os      : "+value("short_ios", v.ShortIOs), "Dropped I/Os    : "+value("drop_ios", v.DropIOs))
		}
		jobError := "unavailable"
		if job.present != nil && !fieldMissing(job.present, "error") {
			jobError = fmt.Sprint(job.Error)
		}
		lines = append(lines, "fio Error       : "+jobError)
		if job.TotalErrors != 0 || job.FirstError != 0 {
			lines = append(lines, fmt.Sprintf("Total Errors    : %d (first error %d)", job.TotalErrors, job.FirstError))
		}
	}
	if c.FIODiagnostics != "" && len(raw) > 0 {
		path, err := retainFailedOutput(c.FIODiagnostics, raw)
		if err != nil {
			cause = errors.Join(cause, fmt.Errorf("retain failed fio output: %w", err))
		} else {
			lines = append(lines, fmt.Sprintf("Raw fio output  : %q", path))
		}
	}
	return fmt.Errorf("invalid fio result for %s QD%d repetition %d: %w\n%s", c.Profile, depth, repetition, cause, strings.Join(lines, "\n"))
}

// CreateTemp is exclusive and mode 0600. Existing files/configuration are never
// overwritten, and this path is reached only for failed workloads with opt-in.
func retainFailedOutput(directory string, data []byte) (path string, err error) {
	if err = os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(directory, "fio-failed-*.json")
	if err != nil {
		return "", err
	}
	path = f.Name()
	_, writeError := f.Write(data)
	err = errors.Join(writeError, f.Close())
	if err != nil {
		return path, err
	}
	return path, nil
}
