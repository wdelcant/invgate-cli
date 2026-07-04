package output

import (
	"fmt"
	"strings"
)

// RecordFormatter renders data vertically — each record gets a header
// separator and its fields printed as aligned key: value pairs.
// Ideal for wide data, nested objects, or inspecting a single resource.
// Invoke with --output record.
type RecordFormatter struct{}

func (f *RecordFormatter) Name() string { return "record" }

func (f *RecordFormatter) Format(data any, cfg FormatConfig) ([]byte, error) {
	v, err := normalize(data)
	if err != nil {
		return nil, err
	}

	items := flattenRecords(v)
	if len(items) == 0 {
		return []byte("(empty)\n"), nil
	}

	// Calculate max key width for alignment.
	maxKey := 0
	for _, item := range items {
		for k := range item {
			if len(k) > maxKey {
				maxKey = len(k)
			}
		}
	}

	var sb strings.Builder
	for i, item := range items {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(fmt.Sprintf("─── Record %d ", i+1))
		sb.WriteString(strings.Repeat("─", maxKey))
		sb.WriteByte('\n')
		keys := sortedKeysStr(item)
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("%-*s  %s\n", maxKey, k, stringify(item[k])))
		}
	}
	return []byte(sb.String()), nil
}

// flattenRecords extracts individual records from the data:
//   - paginated wrapper {results: [...]} → each item
//   - array → each item
//   - single object → one record
func flattenRecords(v any) []map[string]any {
	switch t := v.(type) {
	case map[string]any:
		if arr, ok := t["results"].([]any); ok {
			var out []map[string]any
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					out = append(out, m)
				}
			}
			return out
		}
		return []map[string]any{t}
	case []any:
		var out []map[string]any
		for _, item := range t {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			} else {
				out = append(out, map[string]any{"value": item})
			}
		}
		return out
	default:
		return []map[string]any{{"value": t}}
	}
}

func sortedKeysStr(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple sort without importing sort again
	for i := 0; i < len(keys)-1; i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}
