package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitmera/pkg/git"
	"gitmera/pkg/runner"
	"gitmera/pkg/ui"

	"github.com/spf13/cobra"
)

var (
	checkoutConcurrency int
	checkoutTimeout     time.Duration
	checkoutCreate      bool
)

var checkoutCmd = &cobra.Command{
	Use:          "checkout <branch>",
	Short:        "Switch or create a branch concurrently across all configured repositories",
	Long:         `Reads the workspace configuration and concurrently switches every child repository to the given branch, optionally creating it with -b. Repositories with uncommitted local changes are safely skipped (Safe Fail) without aborting the rest of the run.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		branch := args[0]
		logger := ui.NewSafeLogger(cmd.OutOrStdout(), noColor)

		setup, err := setupCommand(cmd, checkoutConcurrency, checkoutTimeout, "checkout")
		if err != nil {
			return err
		}
		defer setup.cancel()

		action := func(workerCtx context.Context, task runner.RepoTask) (string, bool, error) {
			return checkoutRepo(workerCtx, task.Path, branch, checkoutCreate)
		}

		interactive := isInteractiveMode(cmd.OutOrStdout())
		results := ui.OrchestrateExecution(setup.ctx, setup.tasks, setup.concurrency, false, setup.timeout, action, ui.ExecutionOptions{
			Interactive: interactive,
			ActionLabel: "checking out " + branch,
			Logger:      logger,
		})

		if reportResults(results, logger) || setup.ctx.Err() != nil {
			return fmt.Errorf("checkout failed: one or more repositories failed to checkout %q", branch)
		}

		return nil
	},
}

func checkoutRepo(ctx context.Context, path, branch string, create bool) (warning string, skipped bool, err error) {
	alreadyCloned, verr := git.ValidateDestination(path)
	if verr != nil || !alreadyCloned {
		if verr != nil {
			return "", false, verr
		}
		return "", false, fmt.Errorf("destination path %q does not exist or is not a valid Git repository", path)
	}

	dirty, derr := hasUncommittedTrackedChanges(ctx, path)
	if derr != nil {
		return "", false, derr
	}
	if dirty {
		return "", false, fmt.Errorf("local changes would be overwritten by checkout")
	}

	if !create {
		output, cerr := git.RunGitCommand(ctx, path, "checkout", branch)
		if cerr != nil {
			return string(output), false, cerr
		}
		return "", false, nil
	}

	exists, eerr := localBranchExists(ctx, path, branch)
	if eerr != nil {
		return "", false, eerr
	}

	if exists {
		output, cerr := git.RunGitCommand(ctx, path, "checkout", branch)
		if cerr != nil {
			return string(output), false, cerr
		}
		return fmt.Sprintf("branch %q already exists, switched to it safely", branch), true, nil
	}

	output, cerr := git.RunGitCommand(ctx, path, "checkout", "-b", branch)
	if cerr != nil {
		return string(output), false, cerr
	}
	return "", false, nil
}

func hasUncommittedTrackedChanges(ctx context.Context, path string) (bool, error) {
	output, err := git.RunGitCommand(ctx, path, "status", "--porcelain=v1")
	if err != nil {
		return false, err
	}

	trimmed := strings.TrimRight(string(output), "\n")
	if trimmed == "" {
		return false, nil
	}

	for _, line := range strings.Split(trimmed, "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "??") {
			return true, nil
		}
	}
	return false, nil
}

func localBranchExists(ctx context.Context, path, branch string) (bool, error) {
	_, err := git.RunGitCommand(ctx, path, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if err == nil {
		return true, nil
	}

	return false, nil
}

func init() {
	checkoutCmd.Flags().IntVarP(&checkoutConcurrency, "concurrency", "j", 5, "Maximum number of concurrent checkout operations")
	checkoutCmd.Flags().DurationVar(&checkoutTimeout, "timeout", 2*time.Minute, "Timeout for each individual checkout operation")
	checkoutCmd.Flags().BoolVarP(&checkoutCreate, "branch", "b", false, "Create and switch to a new branch (falls back to a safe switch if it already exists)")
	rootCmd.AddCommand(checkoutCmd)
}
