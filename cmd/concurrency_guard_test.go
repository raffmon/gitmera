package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const concurrencyGuardConfig = `version: "1"
projects:
  example:
    repo: "git@github.com:example/api.git"
    path: "./api"
`

func writeConcurrencyGuardConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitmera.yaml")
	if err := os.WriteFile(path, []byte(concurrencyGuardConfig), 0o644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

var concurrencyGuardCommands = []struct {
	name string
	cmd  *cobra.Command
	args []string
}{
	{name: "status", cmd: statusCmd, args: []string{}},
	{name: "checkout", cmd: checkoutCmd, args: []string{"test-branch"}},
	{name: "push", cmd: pushCmd, args: []string{}},
	{name: "pull", cmd: pullCmd, args: []string{}},
	{name: "clone", cmd: cloneCmd, args: []string{}},
}

func withConcurrencyGuardConfig(t *testing.T) {
	t.Helper()
	prev := cfgFile
	cfgFile = writeConcurrencyGuardConfig(t)
	t.Cleanup(func() { cfgFile = prev })
}

func TestConcurrencyGuard_RejectsZeroAndNegative(t *testing.T) {
	for _, tc := range concurrencyGuardCommands {
		for _, badValue := range []string{"0", "-1"} {
			tc, badValue := tc, badValue
			t.Run(tc.name+"/-j="+badValue, func(t *testing.T) {
				withConcurrencyGuardConfig(t)

				if err := tc.cmd.Flags().Set("concurrency", badValue); err != nil {
					t.Fatalf("failed to set concurrency flag: %v", err)
				}
				t.Cleanup(func() {
					_ = tc.cmd.Flags().Set("concurrency", "5")
				})

				tc.cmd.SetContext(context.Background())
				err := tc.cmd.RunE(tc.cmd, tc.args)
				if err == nil {
					t.Fatalf("expected an error for -j %s, got nil", badValue)
				}
				if !strings.Contains(err.Error(), "concurrency must be a positive integer") {
					t.Fatalf("expected concurrency guard error, got: %v", err)
				}
			})
		}
	}
}

func TestConcurrencyGuard_AllowsValidValues(t *testing.T) {
	for _, tc := range concurrencyGuardCommands {
		for _, validValue := range []string{"1", "5", ""} {
			tc, validValue := tc, validValue
			label := "default"
			if validValue != "" {
				label = "-j=" + validValue
			}
			t.Run(tc.name+"/"+label, func(t *testing.T) {
				withConcurrencyGuardConfig(t)

				if validValue != "" {
					if err := tc.cmd.Flags().Set("concurrency", validValue); err != nil {
						t.Fatalf("failed to set concurrency flag: %v", err)
					}
				}
				t.Cleanup(func() {
					_ = tc.cmd.Flags().Set("concurrency", "5")
				})

				tc.cmd.SetContext(context.Background())
				err := tc.cmd.RunE(tc.cmd, tc.args)
				if err != nil && strings.Contains(err.Error(), "concurrency must be a positive integer") {
					t.Fatalf("guard incorrectly fired for valid value %q: %v", label, err)
				}
			})
		}
	}
}
