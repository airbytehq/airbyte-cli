package output

import (
	"encoding/json"
	"fmt"
	"io"
)

func WriteJSON(w io.Writer, data any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	return nil
}

func WriteJSONCompact(w io.Writer, data any) error {
	enc := json.NewEncoder(w)
	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	return nil
}
