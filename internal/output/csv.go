package output

import (
	"bytes"
	"encoding/csv"
	"fmt"
)

// CSVFormatter renders arrays and objects as RFC 4180 CSV.
// Always writes a header row first, then one row per item.
type CSVFormatter struct{}

func (f *CSVFormatter) Name() string { return "csv" }

func (f *CSVFormatter) Format(data any, cfg FormatConfig) ([]byte, error) {
	v, err := normalize(data)
	if err != nil {
		return nil, err
	}
	rows, headers, err := inferRows(v)
	if err != nil {
		return nil, fmt.Errorf("could not infer csv columns: %w", err)
	}
	if len(cfg.Columns) > 0 {
		headers = cfg.Columns
		rows = projectRows(rows, headers)
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if len(headers) > 0 {
		if err := w.Write(headers); err != nil {
			return nil, fmt.Errorf("could not write csv header: %w", err)
		}
	}
	for _, r := range rows {
		row := make([]string, 0, len(headers))
		for _, h := range headers {
			row = append(row, r[h])
		}
		if err := w.Write(row); err != nil {
			return nil, fmt.Errorf("could not write csv row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("csv write error: %w", err)
	}
	return buf.Bytes(), nil
}