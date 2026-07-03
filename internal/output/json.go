package output

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ANSI color codes for JSON syntax highlighting. Applied only when the
// output is a TTY (FormatConfig.Color) and not in compact mode.
const (
	ansiReset  = "\x1b[0m"
	ansiKey    = "\x1b[34m" // blue
	ansiString = "\x1b[32m" // green
	ansiNumber = "\x1b[33m" // yellow
	ansiBool   = "\x1b[35m" // magenta
	ansiNull   = "\x1b[31m" // red
)

// JSONFormatter renders data as JSON. Pretty-prints with 2-space
// indentation when Color (TTY) is enabled and Compact is false;
// otherwise emits compact single-line JSON. When Color is true the
// pretty output is also syntax-highlighted using ANSI escape codes:
// keys in blue, strings in green, numbers in yellow, booleans in
// magenta, and null in red.
type JSONFormatter struct{}

func (f *JSONFormatter) Name() string { return "json" }

func (f *JSONFormatter) Format(data any, cfg FormatConfig) ([]byte, error) {
	// If the input is already a JSON byte slice or string, re-marshal it
	// through json.Unmarshal so we control indentation/spacing.
	pretty := (cfg.Color || IsTTY()) && !cfg.Compact
	if pretty {
		if cfg.Color {
			return colorizeJSON(data), nil
		}
		return json.MarshalIndent(data, "", "  ")
	}
	return json.Marshal(data)
}

// colorizeJSON renders the provided value as indented (2-space) JSON with
// ANSI color codes applied to each syntactic token. It walks the parsed
// Go value directly so the output mirrors json.MarshalIndent formatting
// (sorted object keys, standard number rendering) while wrapping tokens
// in ANSI colors.
func colorizeJSON(v any) []byte {
	var b strings.Builder
	writeColored(&b, v, 0)
	return []byte(b.String())
}

// writeColored recursively writes v with ANSI colors and 2-space
// indentation at the specified indent level. Numeric types are
// rendered through json.Marshal so output matches MarshalIndent's
// number formatting (e.g. 1 for float64(1.0)).
func writeColored(b *strings.Builder, v any, indent int) {
	switch t := v.(type) {
	case nil:
		b.WriteString(ansiNull)
		b.WriteString("null")
		b.WriteString(ansiReset)
	case bool:
		b.WriteString(ansiBool)
		if t {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		b.WriteString(ansiReset)
	case string:
		b.WriteString(ansiString)
		raw, _ := json.Marshal(t) // produces a properly-escaped JSON string literal
		b.Write(raw)
		b.WriteString(ansiReset)
	case json.Number:
		b.WriteString(ansiNumber)
		b.WriteString(string(t))
		b.WriteString(ansiReset)
	case float32, float64,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		b.WriteString(ansiNumber)
		raw, _ := json.Marshal(t)
		b.Write(raw)
		b.WriteString(ansiReset)
	case map[string]any:
		if len(t) == 0 {
			b.WriteString("{}")
			return
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("{\n")
		for i, k := range keys {
			b.WriteString(strings.Repeat("  ", indent+1))
			raw, _ := json.Marshal(k)
			b.WriteString(ansiKey)
			b.Write(raw)
			b.WriteString(ansiReset)
			b.WriteString(": ")
			writeColored(b, t[k], indent+1)
			if i < len(keys)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(strings.Repeat("  ", indent))
		b.WriteString("}")
	case []any:
		if len(t) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[\n")
		for i, item := range t {
			b.WriteString(strings.Repeat("  ", indent+1))
			writeColored(b, item, indent+1)
			if i < len(t)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(strings.Repeat("  ", indent))
		b.WriteString("]")
	default:
		// Fallback: marshal then add color depending on the resulting JSON token.
		raw, _ := json.Marshal(v)
		if len(raw) > 0 && raw[0] == '"' {
			b.WriteString(ansiString)
			b.Write(raw)
			b.WriteString(ansiReset)
			return
		}
		b.Write(raw)
	}
}

// FormatJSONString is a helper used when the raw response body has
// already been parsed once; it re-encodes the provided JSON string
// according to the format config.
func FormatJSONString(raw string, cfg FormatConfig) ([]byte, error) {
	var v any
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return []byte(raw), nil // best-effort passthrough
	}
	f := &JSONFormatter{}
	return f.Format(v, cfg)
}

// Pretty returns true when the formatter should emit pretty JSON.
func (cfg FormatConfig) Pretty() bool {
	return !cfg.Compact && (cfg.Color || IsTTY())
}

// CompactBytes is a convenience that re-emits already-decoded data
// as compact JSON (no indentation).
func CompactBytes(v any) ([]byte, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("could not encode json: %w", err)
	}
	return []byte(strings.TrimRight(b.String(), "\n")), nil
}