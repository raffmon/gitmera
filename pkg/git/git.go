package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
)

var (
	execCommandMu sync.RWMutex
	execCommand   = exec.CommandContext
)

func getExecCommand() ExecCommandFunc {
	execCommandMu.RLock()
	defer execCommandMu.RUnlock()
	return execCommand
}

var tokenRegex = regexp.MustCompile(`(https?://)([^@\s]+)@`)

func Sanitize(input string) string {
	return tokenRegex.ReplaceAllString(input, "${1}[REDACTED]@")
}

func ValidateDestination(path string) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("destination path cannot be empty")
	}

	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	if !fi.IsDir() {
		return false, fmt.Errorf("destination path %q exists and is a file, not a directory", path)
	}

	gitDir := filepath.Join(path, ".git")
	gfi, err := os.Stat(gitDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("destination path %q exists but is not a valid Git repository (missing .git directory)", path)
		}
		return false, err
	}

	if !gfi.IsDir() {
		return false, fmt.Errorf("destination path %q exists but .git is not a directory", path)
	}

	return true, nil
}

func RunGitCommand(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := getExecCommand()(ctx, "git", args...)
	cmd.Dir = dir

	if len(cmd.Env) == 0 {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ControlMaster=no",
	)

	output, err := cmd.CombinedOutput()
	sanitizedOutput := Sanitize(string(output))

	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return []byte(sanitizedOutput), fmt.Errorf("git command failed: %s: %w", Sanitize(err.Error()), ctxErr)
		}
		return []byte(sanitizedOutput), fmt.Errorf("git command failed: %s: %w", Sanitize(err.Error()), err)
	}

	return []byte(sanitizedOutput), nil
}
