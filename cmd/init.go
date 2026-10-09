package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"gitmera/pkg/config"
	"gitmera/pkg/ui"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var force bool

const defaultConfigPath = ".gitmera.yaml"

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new gitmera workspace configuration",
	Long:  `Creates a .gitmera.yaml configuration file in the current directory, either interactively via a setup wizard or non-interactively with a default template.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		logger := ui.NewSafeLogger(out, noColor)

		wizard := NewWizard(cmd.InOrStdin(), out)

		targetPath := defaultConfigPath
		if cfgFile != "" {
			targetPath = cfgFile
		}

		if _, err := os.Stat(targetPath); err == nil && !force {
			if nonInteractive {
				logger.Print(fmt.Sprintf("Configuration file %s already exists. Aborting (non-interactive mode, use --force to overwrite).\n", targetPath))
				return nil
			}

			confirm, err := wizard.PromptConfirm(fmt.Sprintf("Configuration file %s already exists. Overwrite?", targetPath), false)
			if err != nil {
				return fmt.Errorf("failed to read overwrite confirmation: %w", err)
			}
			if !confirm {
				logger.Print("Initialization aborted.\n")
				return nil
			}
		}

		var cfg config.Config
		cfg.Version = "1"
		cfg.Projects = make(map[string]config.ProjectConfig)

		if nonInteractive {
			cfg.Projects["example"] = config.ProjectConfig{
				Repo: "git@github.com:example/repo.git",
				Path: "./example",
			}
		} else {
			logger.Print("=== Gitmera Configuration Wizard ===\n\n")

			defaultName := "api"
			for {
				name, repo, path, err := promptProject(wizard, logger, cfg.Projects, defaultName)
				if err != nil {
					return err
				}

				cfg.Projects[name] = config.ProjectConfig{
					Repo: repo,
					Path: path,
				}
				logger.LogSuccess(name, "Project added to configuration")
				defaultName = ""

				addAnother, err := wizard.PromptConfirm("Add another repository?", false)
				if err != nil {
					return fmt.Errorf("failed to read add-another confirmation: %w", err)
				}
				if !addAnother {
					break
				}
			}
		}

		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("generated configuration is invalid: %w", err)
		}

		data, err := yaml.Marshal(&cfg)
		if err != nil {
			return fmt.Errorf("failed to format YAML: %w", err)
		}

		if err := os.WriteFile(targetPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write config file: %w", err)
		}

		logger.LogSuccess("gitmera", fmt.Sprintf("Successfully initialized configuration at %s", targetPath))
		return nil
	},
}

func init() {
	initCmd.Flags().BoolVarP(&force, "force", "f", false, "force configuration overwrite without confirmation")
	rootCmd.AddCommand(initCmd)
}

type Wizard struct {
	in  *bufio.Reader
	out io.Writer
}

func NewWizard(in io.Reader, out io.Writer) *Wizard {
	return &Wizard{in: bufio.NewReader(in), out: out}
}

func (w *Wizard) PromptString(promptText string, defaultValue string) (string, error) {
	_, _ = fmt.Fprint(w.out, promptText)
	if defaultValue != "" {
		_, _ = fmt.Fprintf(w.out, " [%s]", defaultValue)
	}
	_, _ = fmt.Fprint(w.out, ": ")

	input, err := w.in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}
	return input, nil
}

func (w *Wizard) PromptConfirm(promptText string, defaultYes bool) (bool, error) {
	suffix := " [y/N]"
	if defaultYes {
		suffix = " [Y/n]"
	}
	_, _ = fmt.Fprintf(w.out, "%s%s: ", promptText, suffix)

	input, err := w.in.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return defaultYes, nil
	}
	return input == "y" || input == "yes", nil
}

func promptProject(wizard *Wizard, logger *ui.SafeLogger, existing map[string]config.ProjectConfig, defaultName string) (name, repo, path string, err error) {
	promptText := "Initial project name"
	if defaultName == "" {
		promptText = "Project name"
	}

	for {
		name, err = wizard.PromptString(promptText, defaultName)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to read project name: %w", err)
		}
		name = strings.TrimSpace(name)

		if name == "" {
			logger.Print("Project name cannot be empty.\n")
			continue
		}
		if _, exists := existing[name]; exists {
			logger.Print(fmt.Sprintf("Project name %q is already used. Choose a different name.\n", name))
			continue
		}
		break
	}

	for {
		repo, err = wizard.PromptString("Git Repository URL", "")
		if err != nil {
			return "", "", "", fmt.Errorf("failed to read repository URL: %w", err)
		}
		if repo != "" {
			break
		}
		logger.Print("Git Repository URL cannot be empty.\n")
	}
	path, err = wizard.PromptString("Local subdirectory path", "./"+name)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read local subdirectory path: %w", err)
	}

	return name, repo, path, nil
}
