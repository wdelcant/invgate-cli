package output

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// TableFormatter renders arrays and objects as ASCII tables using the
// charmbracelet/lipgloss/table package. Column inference:
//   - array of objects → first item's keys become columns,
//   - single object → its top-level keys become columns,
//   - empty array → "(empty)".
type TableFormatter struct{}

func (f *TableFormatter) Name() string { return "table" }

func (f *TableFormatter) Format(data any, cfg FormatConfig) ([]byte, error) {
	// Re-marshal through JSON so we work with []any / map[string]any
	// regardless of the input's original Go type.
	v, err := normalize(data)
	if err != nil {
		return nil, err
	}
	rows, headers, err := inferRows(v)
	if err != nil {
		return nil, fmt.Errorf("could not infer table columns: %w", err)
	}
	if len(cfg.Columns) > 0 {
		headers = cfg.Columns
		rows = projectRows(rows, headers)
	}
	if len(rows) == 0 && len(headers) == 0 {
		return []byte("(empty)\n"), nil
	}
	return renderTable(headers, rows), nil
}

// inferRows derives column ordering and row values from data:
//   - array of maps → headers from first map (sorted), each row from values,
//   - single map → headers from map keys, one row,
//   - empty array → no headers/rows,
//   - anything else → stringified single row.
func inferRows(v any) (rows []map[string]string, headers []string, err error) {
	switch t := v.(type) {
	case []any:
		if len(t) == 0 {
			return nil, nil, nil
		}
		if first, ok := t[0].(map[string]any); ok {
			headers = sortedKeys(first)
		}
		for _, item := range t {
			m, ok := item.(map[string]any)
			if !ok {
				// heterogeneous array — promote to string
				rows = append(rows, map[string]string{"value": stringify(item)})
				if !contains(headers, "value") {
					headers = append(headers, "value")
				}
				continue
			}
			row := make(map[string]string)
			for k, val := range m {
				row[k] = stringify(val)
			}
			rows = append(rows, row)
		}
	case map[string]any:
		// If the response is a paginated wrapper {count, next, previous, results},
		// expand the results array so each item becomes a row.
		if arr, ok := t["results"].([]any); ok && len(arr) > 0 {
			if first, ok := arr[0].(map[string]any); ok {
				headers = sortedKeys(first)
			}
			for _, item := range arr {
				m, ok := item.(map[string]any)
				if !ok {
					rows = append(rows, map[string]string{"value": stringify(item)})
					continue
				}
				row := make(map[string]string)
				for k, val := range m {
					row[k] = stringify(val)
				}
				rows = append(rows, row)
			}
			return rows, headers, nil
		}
		headers = sortedKeys(t)
		row := make(map[string]string)
		for k, val := range t {
			row[k] = stringify(val)
		}
		rows = append(rows, row)
	case nil:
		return nil, nil, nil
	default:
		headers = []string{"value"}
		rows = append(rows, map[string]string{"value": stringify(t)})
	}
	return rows, headers, nil
}

// renderTable produces a clean, borderless column-aligned table with a
// dashed separator under the header row. No Unicode box-drawing characters
// — works on every terminal, including narrow or limited ones.
func renderTable(headers []string, rows []map[string]string) []byte {
	// Calculate column widths (header or data, whichever is wider).
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, h := range headers {
			if l := len(row[h]); l > widths[i] {
				widths[i] = l
			}
		}
	}

	var sb strings.Builder

	// Header row.
	for i, h := range headers {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(fmt.Sprintf("%-*s", widths[i], h))
	}
	sb.WriteByte('\n')

	// Separator.
	for i, w := range widths {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(strings.Repeat("─", w))
	}
	sb.WriteByte('\n')

	// Data rows.
	for _, row := range rows {
		for i, h := range headers {
			if i > 0 {
				sb.WriteString("  ")
			}
			sb.WriteString(fmt.Sprintf("%-*s", widths[i], row[h]))
		}
		sb.WriteByte('\n')
	}

	return []byte(sb.String())
}

// projectRows keeps only the requested columns in the requested order.
func projectRows(rows []map[string]string, columns []string) []map[string]string {
	out := make([]map[string]string, 0, len(rows))
	for _, r := range rows {
		newRow := make(map[string]string, len(columns))
		for _, c := range columns {
			newRow[c] = r[c]
		}
		out = append(out, newRow)
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		// JSON numbers come through as float64.
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case json.Number:
		return string(t)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// normalize converts an arbitrary value into one made of map[string]any
// and []any, so the formatter works on JSON-parsed data.
func normalize(data any) (any, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("could not marshal data: %w", err)
	}
	var v any
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("could not re-decode data: %w", err)
	}
	return v, nil
}