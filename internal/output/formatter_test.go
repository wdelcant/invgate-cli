package output

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"
)

var updateFlag = flag.Bool("update", false, "regenerate golden files")

func TestJSONFormatter(t *testing.T) {
	tests := []struct {
		name string
		data any
		cfg  FormatConfig
		file string
	}{
		{"pretty-object", map[string]any{"id": 1.0, "name": "MacBook"}, FormatConfig{Color: true}, "json-pretty-object.golden"},
		{"compact-object", map[string]any{"id": 1.0, "name": "MacBook"}, FormatConfig{Compact: true}, "json-compact-object.golden"},
		{"array", []any{map[string]any{"a": 1.0}, map[string]any{"b": 2.0}}, FormatConfig{Color: true}, "json-array.golden"},
		{"empty-array", []any{}, FormatConfig{Color: true}, "json-empty-array.golden"},
		{"colored-object", map[string]any{"id": 1.0, "name": "MacBook", "active": true, "missing": nil, "tags": []any{"a", "b"}}, FormatConfig{Color: true}, "json-colored-object.golden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &JSONFormatter{}
			got, err := f.Format(tt.data, tt.cfg)
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			compareGolden(t, tt.file, got)
		})
	}
}

// TestJSONFormatter_ColoredTokens asserts that colored output contains
// ANSI escape codes for each JSON token type: keys (blue), strings
// (green), numbers (yellow), booleans (magenta), and null (red).
func TestJSONFormatter_ColoredTokens(t *testing.T) {
	data := map[string]any{
		"key":    "value",
		"count":  42.0,
		"flag":   true,
		"nope":   nil,
	}
	f := &JSONFormatter{}
	out, err := f.Format(data, FormatConfig{Color: true})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	s := string(out)
	for _, tt := range []struct{ label, code string }{
		{"key color (blue)", ansiKey},
		{"string color (green)", ansiString},
		{"number color (yellow)", ansiNumber},
		{"bool color (magenta)", ansiBool},
		{"null color (red)", ansiNull},
	} {
		if !strings.Contains(s, tt.code) {
			t.Errorf("colored output missing %s (code %q); output=%q", tt.label, tt.code, s)
		}
	}
	// Reset must appear so terminal state is restored.
	if !strings.Contains(s, ansiReset) {
		t.Errorf("colored output missing reset code; output=%q", s)
	}
	// Indentation must remain.
	if !strings.Contains(s, "\n  ") {
		t.Errorf("colored output should still be indented; output=%q", s)
	}
	// The plain (unset) marshal must NOT contain ANSI codes.
	plain, err := (&JSONFormatter{}).Format(data, FormatConfig{Compact: true})
	if err != nil {
		t.Fatalf("plain Format: %v", err)
	}
	if strings.Contains(string(plain), "\x1b[") {
		t.Errorf("compact output must not be colored: %q", string(plain))
	}
}

func TestYAMLFormatter(t *testing.T) {
	data := map[string]any{"id": 1.0, "name": "MacBook", "tags": []any{"laptop", "apple"}}
	f := &YAMLFormatter{}
	got, err := f.Format(data, FormatConfig{})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	compareGolden(t, "yaml-object.golden", got)
	// Validate output is parseable YAML-ish (we just check non-empty).
	if len(got) == 0 {
		t.Error("YAML output should be non-empty")
	}
}

func TestTableFormatter(t *testing.T) {
	tests := []struct {
		name string
		data any
		cfg  FormatConfig
		file string
	}{
		{"array-of-objects",
			[]any{
				map[string]any{"id": 1.0, "name": "MacBook"},
				map[string]any{"id": 2.0, "name": "Dell"},
			},
			FormatConfig{}, "table-array.golden"},
		{"single-object",
			map[string]any{"id": 1.0, "name": "MacBook"},
			FormatConfig{}, "table-object.golden"},
		{"empty-array", []any{}, FormatConfig{}, "table-empty.golden"},
		{"column-filter",
			[]any{
				map[string]any{"id": 1.0, "name": "MacBook", "status": "active"},
				map[string]any{"id": 2.0, "name": "Dell", "status": "retired"},
			},
			FormatConfig{Columns: []string{"id", "name"}}, "table-columns.golden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &TableFormatter{}
			got, err := f.Format(tt.data, tt.cfg)
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			compareGolden(t, tt.file, got)
		})
	}
}

func TestCSVFormatter(t *testing.T) {
	tests := []struct {
		name string
		data any
		cfg  FormatConfig
		file string
	}{
		{"array", []any{
			map[string]any{"id": 1.0, "name": "MacBook"},
			map[string]any{"id": 2.0, "name": "Dell XPS"},
		}, FormatConfig{}, "csv-array.golden"},
		{"column-filter", []any{
			map[string]any{"id": 1.0, "name": "MacBook", "status": "active"},
			map[string]any{"id": 2.0, "name": "Dell XPS", "status": "retired"},
		}, FormatConfig{Columns: []string{"name", "id"}}, "csv-columns.golden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &CSVFormatter{}
			got, err := f.Format(tt.data, tt.cfg)
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			compareGolden(t, tt.file, got)
		})
	}
}

func TestRegistry_Lookup(t *testing.T) {
	for _, name := range []string{"json", "yaml", "table", "csv", "record"} {
		f, err := Get(name)
		if err != nil {
			t.Errorf("Get(%q) error: %v", name, err)
		}
		if f.Name() != name {
			t.Errorf("Get(%q).Name() = %q", name, f.Name())
		}
	}
	_, err := Get("unknown-format")
	if err == nil {
		t.Error("Get(unknown) should error")
	}
	if !strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("Error message should mention unknown format, got: %v", err)
	}
}

func TestFormatJSONString(t *testing.T) {
	raw := `{"id":1,"name":"test"}`
	out, err := FormatJSONString(raw, FormatConfig{})
	if err != nil {
		t.Fatalf("FormatJSONString: %v", err)
	}
	// Compact form should be valid JSON, parseable.
	var v any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Errorf("FormatJSONString output is not valid JSON: %v", err)
	}
}

func TestFormatJSONString_Pretty(t *testing.T) {
	raw := `{"id":1,"name":"test"}`
	out, err := FormatJSONString(raw, FormatConfig{Color: true})
	if err != nil {
		t.Fatalf("FormatJSONString: %v", err)
	}
	// Pretty form should contain a newline.
	if !bytes.Contains(out, []byte("\n")) {
		t.Errorf("pretty output should contain newline, got: %q", out)
	}
}

func TestStringify(t *testing.T) {
	tests := []struct {
		v    any
		want string
	}{
		{nil, ""},
		{"text", "text"},
		{true, "true"},
		{false, "false"},
		{float64(42), "42"},
		{float64(42.5), "42.5"},
	}
	for _, tt := range tests {
		if got := stringify(tt.v); got != tt.want {
			t.Errorf("stringify(%v) = %q, want %q", tt.v, got, tt.want)
		}
	}
}

// TestFormatConfig_Pretty asserts that the Pretty() helper toggles
// indentation based on Compact and Color.
func TestFormatConfig_Pretty(t *testing.T) {
	tests := []struct {
		name string
		cfg  FormatConfig
		want bool
	}{
		{"compact", FormatConfig{Compact: true}, false},
		{"color-tty", FormatConfig{Color: true}, true},
		{"plain-non-tty", FormatConfig{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// IsTTY is environment-dependent; Pretty returns Color||IsTTY&&!Compact.
			// For Color:true case pretty is forced; for plain-non-tty we explicitly
			// check the cfg.Pretty() formula matches the expected value modulo TTY.
			got := tt.cfg.Pretty()
			// Ensure the formula's invariant: when Compact is true it should be false.
			if tt.cfg.Compact && got {
				t.Errorf("Pretty() with Compact should always be false, got %v", got)
			}
			if tt.cfg.Color && !got {
				t.Errorf("Pretty() with Color=true should be true, got %v", got)
			}
			if !tt.cfg.Color && !tt.cfg.Compact {
				// depends on IsTTY; we can't assert strongly but should not panic
				_ = tt.want
			}
		})
	}
}

// TestJSONFormatter_ColoredNestedAssertions verifies nested arrays/objects
// and number rendering consistency with MarshalIndent.
func TestJSONFormatter_ColoredNested(t *testing.T) {
	data := map[string]any{
		"items": []any{
			map[string]any{"id": 1.0, "tags": []any{float64(10.5), float64(20)}},
		},
		"meta": map[string]any{"count": 2.0},
	}
	f := &JSONFormatter{}
	out, err := f.Format(data, FormatConfig{Color: true})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	s := string(out)
	// The colored and plain versions should round-trip to the same JSON
	// once ANSI codes are stripped.
	plain, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	stripped := stripANSI(s)
	if string(plain) != stripped {
		t.Errorf("colored output (ANSI-stripped) should match MarshalIndent\n--- got ---\n%s\n--- want ---\n%s", stripped, string(plain))
	}
}

// stripANSI removes ANSI escape sequences from s.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			// skip until 'm' (the terminator of an SGR sequence)
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// TestTableFormatter_InferRows_Heterogeneous covers the heterogeneous
// array branch in inferRows where items aren't all maps.
func TestTableFormatter_InferRows_Heterogeneous(t *testing.T) {
	f := &TableFormatter{}
	out, err := f.Format([]any{float64(1), "two", true}, FormatConfig{})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if !bytes.Contains(out, []byte("two")) {
		t.Errorf("heterogeneous table should contain 'two': %q", out)
	}
	if !bytes.Contains(out, []byte("value")) {
		t.Errorf("heterogeneous table should use 'value' column: %q", out)
	}
}

// compareGolden writes/refreshes golden files when -update is set, else
// compares to the existing file.
func compareGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := "testdata/" + name
	if *updateFlag {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		// Normalize line endings when writing golden files.
		got = bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n"))
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		// Auto-create initial goldens on first run.
		if os.IsNotExist(err) {
			_ = os.MkdirAll("testdata", 0o755)
			_ = os.WriteFile(path, got, 0o644)
			t.Logf("created golden %s (run with -update to regenerate)", path)
			return
		}
		t.Fatalf("could not read golden file %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		// Normalize line endings before comparison — Go's csv.Writer may
		// produce \r\n on Windows while golden files are always \n.
		got = bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n"))
		want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}