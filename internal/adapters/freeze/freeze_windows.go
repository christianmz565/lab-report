//go:build windows

package freeze

import (
	"time"

	"github.com/UNSAReport/UNSAReport/internal/ports"
)

func getForegroundPGID(_ uintptr) (int, error) {
	return 0, nil
}

func waitForCommand(_ uintptr, _ int, cfg ports.CaptureConfig) {
	timeout := 500 * time.Millisecond
	if cfg.CommandTimeout > 0 {
		timeout = time.Duration(cfg.CommandTimeout) * time.Second / 60
	}
	time.Sleep(timeout)
}
