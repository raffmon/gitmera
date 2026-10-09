package ui_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"gitmera/pkg/ui"
)

func TestNewSafeLogger_NonTTYDisablesColor(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, false)

	if logger.IsColorEnabled() {
		t.Error("expected IsColorEnabled() to be false for a non-TTY writer")
	}

	logger.LogSuccess("api", "success")

	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("expected no ANSI escape sequences in non-TTY output, got: %q", out)
	}
	if !strings.Contains(out, "api") || !strings.Contains(out, "success") {
		t.Errorf("expected output to contain repo name and message, got: %q", out)
	}
}

func TestNewSafeLogger_ForceNoColor(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	if logger.IsColorEnabled() {
		t.Error("expected IsColorEnabled() to be false when forceNoColor is true")
	}

	logger.Print("plain message")
	if got := buf.String(); got != "plain message" {
		t.Errorf("expected raw message to pass through unchanged, got: %q", got)
	}
}

func TestLogErrorBox_ContainsFailureDetails(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	logger.LogErrorBox("web", errors.New("exit status 1"), "fatal: not a git repository")

	out := buf.String()
	for _, want := range []string{"web", "exit status 1", "fatal: not a git repository"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected error box output to contain %q, got: %q", want, out)
		}
	}
}

func TestIsInteractiveTerminal_NonFile(t *testing.T) {
	var buf bytes.Buffer
	if ui.IsInteractiveTerminal(&buf) {
		t.Error("expected IsInteractiveTerminal=false for a non-*os.File writer")
	}
}

func TestIsInteractiveTerminal_File(t *testing.T) {
	if ui.IsInteractiveTerminal(os.Stderr) {
		t.Error("expected IsInteractiveTerminal=false for os.Stderr in a non-TTY test process")
	}
}

func TestNewSafeLogger_OsFileWriter(t *testing.T) {
	logger := ui.NewSafeLogger(os.Stderr, false)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	if logger.IsColorEnabled() {
		t.Error("expected IsColorEnabled=false for os.Stderr in a non-TTY test process")
	}
}

func TestSafeLogger_ConcurrentWritesDoNotInterleave(t *testing.T) {
	var buf bytes.Buffer
	logger := ui.NewSafeLogger(&buf, true)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			logger.LogSuccess(fmt.Sprintf("repo%d", n), "success")
		}(i)
	}
	wg.Wait()

	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != goroutines {
		t.Fatalf("expected %d output lines, got %d; output: %q", goroutines, len(lines), out)
	}

	seen := make(map[string]bool, goroutines)
	for _, line := range lines {
		if !strings.Contains(line, ": success") {
			t.Errorf("line is corrupted/interleaved (missing expected suffix): %q", line)
			continue
		}
		seen[line] = true
	}

	for i := 0; i < goroutines; i++ {
		repo := fmt.Sprintf("repo%d", i)
		found := false
		for line := range seen {
			if strings.Contains(line, repo) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected a line for %q, not found in output", repo)
		}
	}
}
