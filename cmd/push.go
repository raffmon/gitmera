package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gitmera/pkg/git"
	"gitmera/pkg/runner"
	"gitmera/pkg/ui"

	"github.com/spf13/cobra"
)

var (
	pushConcurrency int
	pushTimeout     time.Duration
)

var pushCmd = &cobra.Command{
	Use:          "push",
	Short:        "Smart-push the current branch concurrently across all configured repositories",
	Long:         `Reads the workspace configuration and concurrently pushes the current branch to origin in every child repository, skipping repositories that are already up-to-date or behind/diverged, and auto-configuring upstream tracking for branches that don't have it yet.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := ui.NewSafeLogger(cmd.OutOrStdout(), noColor)

		setup, err := setupCommand(cmd, pushConcurrency, pushTimeout, "push")
		if err != nil {
			return err
		}
		defer setup.cancel()

		action := func(workerCtx context.Context, task runner.RepoTask) (string, bool, error) {
			return pushRepo(workerCtx, task.Path)
		}

		interactive := isInteractiveMode(cmd.OutOrStdout())
		results := ui.OrchestrateExecution(setup.ctx, setup.tasks, setup.concurrency, false, setup.timeout, action, ui.ExecutionOptions{
			Interactive: interactive,
			ActionLabel: "pushing",
			Logger:      logger,
		})

		if reportResults(results, logger) || setup.ctx.Err() != nil {
			return fmt.Errorf("push failed: one or more repositories failed to push")
		}

		return nil
	},
}

func pushRepo(ctx context.Context, path string) (reason string, skipped bool, err error) {
	alreadyCloned, verr := git.ValidateDestination(path)
	if verr != nil || !alreadyCloned {
		if verr != nil {
			return "", false, verr
		}
		return "", false, fmt.Errorf("destination path %q does not exist or is not a valid Git repository", path)
	}

	branch, berr := currentBranch(ctx, path)
	if berr != nil {
		return "", false, berr
	}

	hasUpstream, uerr := hasUpstreamConfigured(ctx, path)
	if uerr != nil {
		return "", false, uerr
	}

	if !hasUpstream {
		output, perr := git.RunGitCommand(ctx, path, "push", "--set-upstream", "origin", branch)
		if perr != nil {
			return string(output), false, perr
		}
		return "", false, nil
	}

	ahead, behind, derr := aheadBehindCount(ctx, path)
	if derr != nil {
		return "", false, derr
	}

	if behind > 0 {
		if ahead > 0 {
			return fmt.Sprintf("skipped: diverged from remote (ahead %d, behind %d) — manual merge/rebase required", ahead, behind), true, nil
		}
		return fmt.Sprintf("skipped: behind remote by %d commit(s) — pull or rebase before pushing", behind), true, nil
	}

	if ahead == 0 {
		return "skipped: already up-to-date with remote", true, nil
	}

	output, perr := git.RunGitCommand(ctx, path, "push", "origin", branch)
	if perr != nil {
		return string(output), false, perr
	}
	return "", false, nil
}

func currentBranch(ctx context.Context, path string) (string, error) {
	output, err := git.RunGitCommand(ctx, path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(string(output))
	if branch == "" || branch == "HEAD" {
		return "", fmt.Errorf("cannot push from a detached HEAD state")
	}
	return branch, nil
}

func hasUpstreamConfigured(ctx context.Context, path string) (bool, error) {
	_, err := git.RunGitCommand(ctx, path, "rev-parse", "--symbolic-full-name", "@{u}")
	if err != nil {
		return false, nil
	}
	return true, nil
}

func aheadBehindCount(ctx context.Context, path string) (ahead, behind int, err error) {
	output, rerr := git.RunGitCommand(ctx, path, "rev-list", "--count", "--left-right", "HEAD...@{u}")
	if rerr != nil {
		return 0, 0, rerr
	}

	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected rev-list output: %q", string(output))
	}

	ahead, errA := strconv.Atoi(fields[0])
	behind, errB := strconv.Atoi(fields[1])
	if errA != nil || errB != nil {
		return 0, 0, fmt.Errorf("failed to parse rev-list ahead/behind counts: %q", string(output))
	}

	return ahead, behind, nil
}

func init() {
	pushCmd.Flags().IntVarP(&pushConcurrency, "concurrency", "j", 5, "Maximum number of concurrent push operations")
	pushCmd.Flags().DurationVar(&pushTimeout, "timeout", 2*time.Minute, "Timeout for each individual push operation")
	rootCmd.AddCommand(pushCmd)
}
