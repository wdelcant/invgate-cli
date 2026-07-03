// Package config handles YAML configuration stored under the XDG
// config directory. Secrets are NEVER written to the config file —
// only base_url, spec_path, output, and timeout.
//
// Configuration loading and saving is backed by spf13/viper, matching
// the design document. Viper provides file + env + defaults cascade
// and YAML (de)serialization out of the box.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// Config holds the non-secret CLI configuration persisted to disk.
type Config struct {
	BaseURL  string        `yaml:"base_url"`
	SpecPath string        `yaml:"spec_path"`
	Output   string        `yaml:"output"`
	Timeout  time.Duration `yaml:"timeout"`
}

// Defaults applied when a value is missing.
const (
	DefaultOutput  = "json"
	DefaultTimeout = 30 * time.Second
)

// envBindings maps a config key to the env var that overrides it.
// Viper's AutomaticEnv already derives INVGATE_BASE_URL from key
// base_url (PREFIX + _ + UPPER(key)), so the bindings here are just
// explicit for documentation; they also serve as a fallback hooks if
// the key separators ever change.
var envBindings = map[string]string{
	"base_url":  "INVGATE_BASE_URL",
	"spec_path": "INVGATE_SPEC",
	"output":    "INVGATE_OUTPUT",
	"timeout":   "INVGATE_TIMEOUT",
}

// ConfigDir returns the resolved config directory honoring XDG.
// Order: $XDG_CONFIG_HOME/invgate-cli, else $HOME/.config/invgate-cli.
func ConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "invgate-cli"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "invgate-cli"), nil
}

// ConfigPath returns the full path to config.yaml.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// DefaultSpecPath returns the default location for a spec file
// under the config directory.
func DefaultSpecPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "spec.json"), nil
}

// defaults returns a Config pre-populated with default values.
func defaults() *Config {
	return &Config{
		Output:  DefaultOutput,
		Timeout: DefaultTimeout,
	}
}

// newViper returns a fresh viper instance configured with the invgate
// env prefix and automatic env var lookup, ready to read or write the
// XDG config file. The caller decides whether to ReadInConfig.
func newViper() (*viper.Viper, error) {
	v := viper.NewWithOptions(viper.KeyDelimiter("."))
	v.SetConfigType("yaml")
	v.SetEnvPrefix("INVGATE")
	v.AutomaticEnv()
	// Bind explicit env names so viper looks them up first.
	for key, env := range envBindings {
		_ = v.BindEnv(key, env)
	}
	// Defaults so missing files don't produce empty values.
	v.SetDefault("output", DefaultOutput)
	v.SetDefault("timeout", DefaultTimeout)
	return v, nil
}

// Load reads the YAML config file (if it exists) and applies env var
// overrides via viper's automatic env lookup. Missing files are not
// errors — defaults are applied. Env vars always win over file values.
func Load() (*Config, error) {
	cfg := defaults()

	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	v, err := newViper()
	if err != nil {
		return nil, err
	}
	v.SetConfigFile(path)
	if rerr := v.ReadInConfig(); rerr != nil {
		// Missing file is non-fatal — defaults + env apply.
		if _, ok := rerr.(viper.ConfigFileNotFoundError); !ok && !os.IsNotExist(rerr) {
			return nil, fmt.Errorf("could not read config file %s: %w", path, rerr)
		}
	}

	cfg.BaseURL = v.GetString("base_url")
	cfg.SpecPath = v.GetString("spec_path")
	cfg.Output = v.GetString("output")
	if cfg.Output == "" {
		cfg.Output = DefaultOutput
	}
	cfg.Timeout = v.GetDuration("timeout")
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	return cfg, nil
}

// Save writes the config to disk as YAML. Only non-secret fields are
// written. The config directory is created if it does not exist.
func (c *Config) Save() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("could not create config directory %s: %w", dir, err)
	}
	path := filepath.Join(dir, "config.yaml")
	return c.SaveToPath(path)
}

// LoadFromPath loads a config from an explicit path (used in tests).
// Env vars still override file values via viper's automatic env.
func LoadFromPath(path string) (*Config, error) {
	v, err := newViper()
	if err != nil {
		return nil, err
	}
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("could not read config file %s: %w", path, err)
	}
	output := v.GetString("output")
	if output == "" {
		output = DefaultOutput
	}
	timeout := v.GetDuration("timeout")
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return &Config{
		BaseURL:  v.GetString("base_url"),
		SpecPath: v.GetString("spec_path"),
		Output:   output,
		Timeout:  timeout,
	}, nil
}

// SaveToPath writes the config to an explicit path (used in tests).
func (c *Config) SaveToPath(path string) error {
	v, err := newViper()
	if err != nil {
		return err
	}
	// viper recommends Set defaults then overrides; here we set the
	// explicit values chosen by the caller.
	v.Set("base_url", c.BaseURL)
	v.Set("spec_path", c.SpecPath)
	v.Set("output", c.Output)
	// Duration stored as a Go duration string so it round-trips via GetDuration.
	v.Set("timeout", c.Timeout.String())
	if err := v.WriteConfigAs(path); err != nil {
		return fmt.Errorf("could not write config file %s: %w", path, err)
	}
	return nil
}