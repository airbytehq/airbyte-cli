package output

import (
	"fmt"
	"io"
	"os"
)

func Write(data any, format, outputPath string) error {
	var w io.Writer = os.Stdout
	if outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("opening output file: %w", err)
		}
		defer func() { _ = f.Close() }()
		w = f
	}

	switch format {
	case "table":
		return WriteTable(w, data)
	default:
		return WriteJSON(w, data)
	}
}

func WriteError(data any) {
	_ = WriteJSON(os.Stderr, data)
}
