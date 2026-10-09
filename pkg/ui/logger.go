
package ui

import (
	"fmt"
	"io"
	"os"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/mattn/go-isatty"
)

type SafeLogger struct {
	mu      sync.Mutex
	out     io.Writer
	cpw     *colorprofile.Writer
	noColor bool
}

func NewSafeLogger(out io.Writer, forceNoColor bool) *SafeLogger {
	isTerminal := false
	if f, ok := out.(*os.File); ok {
		isTerminal = isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
	}

	profile := colorprofile.Detect(out, os.Environ())
	if forceNoColor || !isTerminal {
		profile = colorprofile.NoTTY
	}

	cpw := &colorprofile.Writer{
		Forward: out,
		Profile: profile,
	}

	return &SafeLogger{
		out:     out,
		cpw:     cpw,
		noColor: forceNoColor || !isTerminal,
	}
}

func (l *SafeLogger) IsColorEnabled() bool {
	return !l.noColor
}

func IsInteractiveTerminal(out io.Writer) bool {
	f, ok := out.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

func (l *SafeLogger) LogSuccess(repoName, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	checkmark := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("✓")
	repoStyle := lipgloss.NewStyle().Bold(true).Render(repoName)

	_, _ = fmt.Fprintf(l.cpw, "%s %s: %s\n", checkmark, repoStyle, message)
}

func (l *SafeLogger) LogErrorBox(repoName string, err error, stderr string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")).
		Bold(true)

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("9")).
		Padding(0, 1)

	header := headerStyle.Render(fmt.Sprintf("FAILURE: %s", repoName))
	body := fmt.Sprintf("Error: %v\n\n%s", err, stderr)

	content := fmt.Sprintf("%s\n%s", header, body)
	box := borderStyle.Render(content)

	_, _ = fmt.Fprintln(l.cpw, box)
}

func (l *SafeLogger) Print(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = fmt.Fprint(l.cpw, msg)
}
