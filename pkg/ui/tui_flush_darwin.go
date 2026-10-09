//go:build darwin

package ui

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const flushRead = 0x1

func FlushStdin() {
	time.Sleep(10 * time.Millisecond)
	fd := int(os.Stdin.Fd())
	_ = unix.IoctlSetInt(fd, unix.TIOCFLUSH, flushRead)
}
