package output

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]string{"id": "1", "name": "test"}

	if err := WriteJSON(&buf, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parsing output: %v", err)
	}
	if result["id"] != "1" {
		t.Errorf("expected id='1', got %q", result["id"])
	}

	if !strings.Contains(buf.String(), "\n  ") {
		t.Error("expected pretty-printed JSON with indentation")
	}
}

func TestWriteJSONCompact(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]string{"id": "1"}

	if err := WriteJSONCompact(&buf, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := strings.TrimSpace(buf.String())
	if strings.Contains(output, "\n") {
		t.Errorf("expected compact JSON without newlines in value, got %q", output)
	}
}

func TestWriteToFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "output.json")
	data := map[string]string{"key": "value"}

	if err := Write(data, tmpFile); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(content, &result); err != nil {
		t.Fatalf("parsing output: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("expected key='value', got %q", result["key"])
	}
}
