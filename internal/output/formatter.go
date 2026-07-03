// Package output provides formatters for CLI response rendering.
// A registry maps format names to Formatter implementations. The
// format is resolved at runtime from --output / env / config and
// selected via the registry.
package output

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
)

// FormatConfig controls formatting options shared by all formatters.
type FormatConfig struct {
	Columns []string // restrict/reorder columns (table/csv)
	Compact bool     // force compact output (no indentation / colors)
	Color   bool     // enable color output (TTY only by default)
}

// Formatter renders arbitrary data into bytes for stdout.
type Formatter interface {
	Name() string
	Format(data any, cfg FormatConfig) ([]byte, error)
}

// Registry maps format names to their Formatter implementation.
// Use Register in package init blocks or test setup to add new formats.
var Registry = map[string]Formatter{}

// Register adds a formatter to the registry, keyed by Name().
func Register(f Formatter) {
	Registry[f.Name()] = f
}

// Get returns the formatter for the given name or an error if unknown.
func Get(name string) (Formatter, error) {
	f, ok := Registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown output format %q (available: %s)", name, availableFormats())
	}
	return f, nil
}

// IsTTY reports whether stdout appears to be an interactive terminal.
func IsTTY() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

func availableFormats() string {
	names := make([]string, 0, len(Registry))
	for name := range Registry {
		names = append(names, name)
	}
	return fmt.Sprint(names)
}

func init() {
	Register(&JSONFormatter{})
	Register(&YAMLFormatter{})
	Register(&TableFormatter{})
	Register(&CSVFormatter{})
}