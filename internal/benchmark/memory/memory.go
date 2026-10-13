package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"vmbench/internal/benchmark/cpu"
	"vmbench/internal/config"
	"vmbench/internal/system"
)

func Run(ctx context.Context, c config.Config, integrity bool) ([]json.RawMessage, error) {
	n, e := config.Bytes(c.Size)
	if e != nil {
		return nil, e
	}
	available := system.Detect().AvailableMemory
	if available <= 0 {
		return nil, fmt.Errorf("available memory could not be determined")
	}
	if n > available/2 {
		return nil, fmt.Errorf("dataset exceeds 50%% available memory allocation")
	}
	mode := "ram"
	if integrity {
		mode = "integrity"
	}
	return cpu.Run(ctx, c, mode)
}
