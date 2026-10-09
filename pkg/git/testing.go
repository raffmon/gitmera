package git

import (
	"context"
	"os/exec"
)

type ExecCommandFunc func(ctx context.Context, name string, args ...string) *exec.Cmd

func SetExecCommandForTest(fn ExecCommandFunc) (restore func()) {
	execCommandMu.Lock()
	prev := execCommand
	execCommand = fn
	execCommandMu.Unlock()
	return func() {
		execCommandMu.Lock()
		execCommand = prev
		execCommandMu.Unlock()
	}
}
