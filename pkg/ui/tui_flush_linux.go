//go:build linux

package ui

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func FlushStdin() {
	time.Sleep(10 * time.Millisecond)
	fd := int(os.Stdin.Fd())
	_ = unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIFLUSH)
}
