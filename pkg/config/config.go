
package config

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type ProjectConfig struct {
	Repo string `yaml:"repo"`
	Path string `yaml:"path"`
}

type Config struct {
	Version     string                   `yaml:"version"`
	Concurrency *int                     `yaml:"concurrency,omitempty"`
	Timeout     *string                  `yaml:"timeout,omitempty"`
	Projects    map[string]ProjectConfig `yaml:"projects"`
}

var gitURIRegex = regexp.MustCompile(`^(https?|git|ssh|file)://.+|^(git@|.+@)?[a-zA-Z0-9.-]+:[^:].*$`)

func Load(r io.Reader) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)

	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.Version == "" {
		return fmt.Errorf("missing version field")
	}
	if c.Concurrency != nil {
		if *c.Concurrency < 1 {
			return fmt.Errorf("concurrency must be a positive integer greater than or equal to 1")
		}
	}
	if c.Timeout != nil {
		d, err := time.ParseDuration(*c.Timeout)
		if err != nil {
			return fmt.Errorf("invalid timeout format: %w", err)
		}
		if d <= 0 {
			return fmt.Errorf("timeout must be a positive duration")
		}
	}
	if len(c.Projects) == 0 {
		return fmt.Errorf("no projects configured")
	}

	for name, proj := range c.Projects {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("project name cannot be empty")
		}
		if proj.Repo == "" {
			return fmt.Errorf("project %q: repo URI cannot be empty", name)
		}
		if !gitURIRegex.MatchString(proj.Repo) {
			return fmt.Errorf("project %q: invalid Git URI format: %q", name, proj.Repo)
		}
		if proj.Path == "" {
			return fmt.Errorf("project %q: local path cannot be empty", name)
		}
		if filepath.IsAbs(proj.Path) {
			return fmt.Errorf("project %q: local path %q must be relative", name, proj.Path)
		}

		cleanPath := filepath.Clean(proj.Path)
		if cleanPath == ".." || strings.HasPrefix(cleanPath, "../") || strings.HasPrefix(cleanPath, "..\\") {
			return fmt.Errorf("project %q: path traversal detected in local path: %q", name, proj.Path)
		}
	}
	return nil
}
