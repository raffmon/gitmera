package cmd

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitmera/pkg/git"
	"gitmera/pkg/runner"
	"gitmera/pkg/ui"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

var (
	statusConcurrency int
	statusTimeout     time.Duration
	statusFetch       bool
)

type repoStatus struct {
	Name        string
	Branch      string
	Detached    bool
	Status      string
	Ahead       int
	Behind      int
	HasUpstream bool
	Diverged    bool
	UpToDate    bool
	Err         error
}

var statusCmd = &cobra.Command{
	Use:          "status",
	Short:        "Show a unified status summary across all configured repositories",
	Long:         `Reads the workspace configuration and concurrently checks git status in all child repositories, displaying a colorized table summarizing branch, modification, and ahead/behind divergence state.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		setup, err := setupCommand(cmd, statusConcurrency, statusTimeout, "status")
		if err != nil {
			return err
		}
		defer setup.cancel()

		statuses := make(map[string]repoStatus, len(setup.tasks))
		var statusMu sync.Mutex

		action := func(workerCtx context.Context, task runner.RepoTask) (string, bool, error) {
			rs := collectRepoStatus(workerCtx, task.Name, task.Path, statusFetch)
			statusMu.Lock()
			statuses[task.Name] = rs
			statusMu.Unlock()
			return "", false, nil
		}

		runner.ExecuteTasks(setup.ctx, setup.tasks, setup.concurrency, false, setup.timeout, action, nil)

		ordered := make([]repoStatus, 0, len(setup.tasks))
		for _, task := range setup.tasks {
			ordered = append(ordered, statuses[task.Name])
		}

		var tableBuf strings.Builder
		renderStatusTable(&tableBuf, ordered)
		renderMissingDetails(&tableBuf, ordered)

		logger := ui.NewSafeLogger(cmd.OutOrStdout(), noColor)
		logger.Print(tableBuf.String())

		return nil
	},
}

func init() {
	statusCmd.Flags().IntVarP(&statusConcurrency, "concurrency", "j", 5, "Maximum number of concurrent status check operations")
	statusCmd.Flags().DurationVar(&statusTimeout, "timeout", 2*time.Minute, "Timeout for each individual status check operation")
	statusCmd.Flags().BoolVarP(&statusFetch, "fetch", "f", false, "Perform a concurrent git fetch in all repositories before computing status")
	rootCmd.AddCommand(statusCmd)
}

func collectRepoStatus(ctx context.Context, name, path string, doFetch bool) repoStatus {
	rs := repoStatus{Name: name}

	alreadyCloned, err := git.ValidateDestination(path)
	if err != nil || !alreadyCloned {
		rs.Status = "Missing"
		rs.Err = err
		return rs
	}

	if doFetch {
		_, _ = git.RunGitCommand(ctx, path, "fetch")
	}

	output, err := git.RunGitCommand(ctx, path, "status", "--porcelain=v1", "-b")
	if err != nil {
		rs.Status = "Missing"
		rs.Err = err
		return rs
	}

	lines := strings.Split(strings.TrimRight(string(output), "\n"), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "## ") {
		rs.Status = "Missing"
		rs.Err = fmt.Errorf("unexpected git status output: missing branch header")
		return rs
	}

	parseBranchLine(lines[0], &rs)

	if len(lines) > 1 {
		rs.Status = "Modified"
	} else {
		rs.Status = "Clean"
	}

	if rs.HasUpstream {
		computeAheadBehind(ctx, path, &rs)
	}

	return rs
}

func parseBranchLine(line string, rs *repoStatus) {
	header := strings.TrimPrefix(line, "## ")

	if strings.HasPrefix(header, "HEAD (no branch)") {
		rs.Detached = true
		rs.Branch = "(detached HEAD)"
		rs.HasUpstream = false
		return
	}

	if idx := strings.Index(header, " ["); idx != -1 {
		header = header[:idx]
	}

	if strings.Contains(header, "...") {
		parts := strings.SplitN(header, "...", 2)
		rs.Branch = parts[0]
		rs.HasUpstream = true
		return
	}

	if strings.HasPrefix(header, "No commits yet on ") {
		rs.Branch = strings.TrimPrefix(header, "No commits yet on ")
	} else {
		rs.Branch = header
	}
	rs.HasUpstream = false
}

func computeAheadBehind(ctx context.Context, path string, rs *repoStatus) {
	output, err := git.RunGitCommand(ctx, path, "rev-list", "--count", "--left-right", "HEAD...@{u}")
	if err != nil {
		rs.HasUpstream = false
		return
	}

	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) != 2 {
		rs.HasUpstream = false
		return
	}

	ahead, errA := strconv.Atoi(fields[0])
	behind, errB := strconv.Atoi(fields[1])
	if errA != nil || errB != nil {
		rs.HasUpstream = false
		return
	}

	rs.Ahead = ahead
	rs.Behind = behind
	rs.Diverged = ahead > 0 && behind > 0
	rs.UpToDate = ahead == 0 && behind == 0
}

var (
	statusRepoStyle     = lipgloss.NewStyle().Bold(true)
	statusBranchStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	statusCleanStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	statusModifiedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	statusMissingStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	statusGrayStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	statusHeaderStyle = lipgloss.NewStyle().Bold(true)
)

func renderStatusTable(w io.Writer, statuses []repoStatus) {
	headers := []string{"REPOSITORY", "BRANCH", "STATUS", "AHEAD/BEHIND"}

	rows := make([][]string, 0, len(statuses))
	for _, rs := range statuses {
		rows = append(rows, []string{
			statusRepoStyle.Render(rs.Name),
			renderBranchCell(rs),
			renderStatusCell(rs),
			renderAheadBehindCell(rs),
		})
	}

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = lipgloss.Width(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if cw := lipgloss.Width(cell); cw > widths[i] {
				widths[i] = cw
			}
		}
	}

	var sb strings.Builder

	for i, h := range headers {
		sb.WriteString(padRight(statusHeaderStyle.Render(h), widths[i]))
		if i < len(headers)-1 {
			sb.WriteString("  ")
		}
	}
	sb.WriteString("\n")

	for _, row := range rows {
		for i, cell := range row {
			sb.WriteString(padRight(cell, widths[i]))
			if i < len(row)-1 {
				sb.WriteString("  ")
			}
		}
		sb.WriteString("\n")
	}

	_, _ = fmt.Fprint(w, sb.String())
}

func renderMissingDetails(w io.Writer, statuses []repoStatus) {
	for _, rs := range statuses {
		if rs.Status == "Missing" && rs.Err != nil {
			_, _ = fmt.Fprintf(w, "  ↳ %s: %s\n", rs.Name, rs.Err.Error())
		}
	}
}

func padRight(str string, width int) string {
	visualWidth := lipgloss.Width(str)
	if visualWidth >= width {
		return str
	}
	return str + strings.Repeat(" ", width-visualWidth)
}

func renderBranchCell(rs repoStatus) string {
	if rs.Status == "Missing" {
		return statusGrayStyle.Render("-")
	}
	return statusBranchStyle.Render(rs.Branch)
}

func renderStatusCell(rs repoStatus) string {
	switch rs.Status {
	case "Clean":
		return statusCleanStyle.Render("Clean")
	case "Modified":
		return statusModifiedStyle.Render("Modified")
	case "Missing":
		return statusMissingStyle.Render("Missing")
	default:
		return statusGrayStyle.Render(rs.Status)
	}
}

func renderAheadBehindCell(rs repoStatus) string {
	if rs.Status == "Missing" {
		return statusGrayStyle.Render("-")
	}
	if !rs.HasUpstream {
		return statusGrayStyle.Render("no upstream")
	}
	switch {
	case rs.Diverged:
		return statusMissingStyle.Render(fmt.Sprintf("diverged (ahead %d, behind %d)", rs.Ahead, rs.Behind))
	case rs.Ahead > 0:
		return statusCleanStyle.Render(fmt.Sprintf("ahead %d", rs.Ahead))
	case rs.Behind > 0:
		return statusModifiedStyle.Render(fmt.Sprintf("behind %d", rs.Behind))
	default:
		return statusGrayStyle.Render("up-to-date")
	}
}
