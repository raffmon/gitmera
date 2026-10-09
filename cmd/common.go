package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"gitmera/pkg/config"
	"gitmera/pkg/runner"
	"gitmera/pkg/ui"

	"github.com/spf13/cobra"
)

type commandSetup struct {
	cfg         *config.Config
	concurrency int
	timeout     time.Duration
	ctx         context.Context
	cancel      context.CancelFunc
	tasks       []runner.RepoTask
}

func setupCommand(cmd *cobra.Command, concurrencyFlag int, timeoutFlag time.Duration, action string) (*commandSetup, error) {
	configPath, err := resolveConfigPath(cfgFile)
	if err != nil {
		return nil, err
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}

	concurrency, err := resolveConcurrency(cmd, concurrencyFlag, cfg)
	if err != nil {
		return nil, err
	}

	timeout, err := resolveTimeout(cmd, timeoutFlag, cfg)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), globalTimeoutFor(timeout))

	return &commandSetup{
		cfg:         cfg,
		concurrency: concurrency,
		timeout:     timeout,
		ctx:         ctx,
		cancel:      cancel,
		tasks:       buildSortedTasks(cfg, action),
	}, nil
}

func loadConfig(configPath string) (*config.Config, error) {
	f, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open configuration file %q: %w", configPath, err)
	}
	defer func() { _ = f.Close() }()

	cfg, err := config.Load(f)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration file %q: %w", configPath, err)
	}
	return cfg, nil
}

func resolveConcurrency(cmd *cobra.Command, flagValue int, cfg *config.Config) (int, error) {
	concurrency := 5
	if cmd.Flags().Changed("concurrency") {
		concurrency = flagValue
	} else if cfg.Concurrency != nil {
		concurrency = *cfg.Concurrency
	}

	if concurrency < 1 {
		return 0, fmt.Errorf("concurrency must be a positive integer greater than or equal to 1, got %d", concurrency)
	}
	return concurrency, nil
}

func resolveTimeout(cmd *cobra.Command, flagValue time.Duration, cfg *config.Config) (time.Duration, error) {
	if cmd.Flags().Changed("timeout") {
		return flagValue, nil
	}
	if cfg.Timeout != nil {
		parsed, err := time.ParseDuration(*cfg.Timeout)
		if err != nil {
			return 0, fmt.Errorf("invalid timeout %q in configuration: %w", *cfg.Timeout, err)
		}
		return parsed, nil
	}
	return 2 * time.Minute, nil
}

func globalTimeoutFor(timeout time.Duration) time.Duration {
	global := 5 * timeout
	if global > 10*time.Minute {
		return 10 * time.Minute
	}
	return global
}

func buildSortedTasks(cfg *config.Config, action string) []runner.RepoTask {
	names := make([]string, 0, len(cfg.Projects))
	for name := range cfg.Projects {
		names = append(names, name)
	}
	sort.Strings(names)

	tasks := make([]runner.RepoTask, 0, len(names))
	for _, name := range names {
		proj := cfg.Projects[name]
		tasks = append(tasks, runner.RepoTask{
			Name:   name,
			URI:    proj.Repo,
			Path:   proj.Path,
			Action: action,
		})
	}
	return tasks
}

func isCancellationErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func reportResults(results []runner.TaskResult, logger *ui.SafeLogger) bool {
	hasRealError := false
	for _, res := range results {
		switch {
		case res.Err != nil:
			if !isCancellationErr(res.Err) {
				hasRealError = true
				logger.LogErrorBox(res.RepoName, res.Err, res.Stderr)
			}
		case res.Skipped && res.Stderr != "":
			logger.Print(fmt.Sprintf("  ↳ %s: %s\n", res.RepoName, res.Stderr))
		}
	}
	return hasRealError
}
