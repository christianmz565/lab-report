//go:build !windows

package freeze

import (
	"time"
	"unsafe"

	"github.com/UNSAReport/UNSAReport/internal/ports"
	"golang.org/x/sys/unix"
)

func getForegroundPGID(fd uintptr) (int, error) {
	var pgid int32
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, unix.TIOCGPGRP, uintptr(unsafe.Pointer(&pgid)))
	if errno != 0 {
		return 0, errno
	}
	return int(pgid), nil
}

func waitForCommand(fd uintptr, shellPGID int, cfg ports.CaptureConfig) {
	time.Sleep(50 * time.Millisecond)

	timeout := 30 * time.Second
	if cfg.CommandTimeout > 0 {
		timeout = time.Duration(cfg.CommandTimeout) * time.Second
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pgid, err := getForegroundPGID(fd)
		if err == nil && pgid == shellPGID {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}
