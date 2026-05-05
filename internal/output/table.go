package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

func WriteTable(w io.Writer, data any) error {
	rows, err := toRows(data)
	if err != nil {
		return WriteJSON(w, data)
	}
	if len(rows) == 0 {
		return WriteJSON(w, data)
	}

	columns := extractColumns(rows)
	if len(columns) == 0 {
		return WriteJSON(w, data)
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)

	headers := make([]string, len(columns))
	for i, col := range columns {
		headers[i] = strings.ToUpper(col)
	}
	if _, err := fmt.Fprintln(tw, strings.Join(headers, "\t")); err != nil {
		return err
	}

	for _, row := range rows {
		vals := make([]string, len(columns))
		for i, col := range columns {
			vals[i] = formatValue(row[col])
		}
		if _, err := fmt.Fprintln(tw, strings.Join(vals, "\t")); err != nil {
			return err
		}
	}

	return tw.Flush()
}

func toRows(data any) ([]map[string]any, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	var rows []map[string]any
	if err := json.Unmarshal(b, &rows); err == nil {
		return rows, nil
	}

	var single map[string]any
	if err := json.Unmarshal(b, &single); err == nil {
		return []map[string]any{single}, nil
	}

	return nil, fmt.Errorf("cannot convert to table rows")
}

func extractColumns(rows []map[string]any) []string {
	seen := make(map[string]bool)
	var columns []string
	for _, row := range rows {
		for k := range row {
			if !seen[k] {
				seen[k] = true
				columns = append(columns, k)
			}
		}
	}
	sort.Strings(columns)
	return columns
}

func formatValue(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}
