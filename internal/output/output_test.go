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

func TestWriteTable(t *testing.T) {
	var buf bytes.Buffer
	data := []map[string]any{
		{"id": "1", "name": "alpha"},
		{"id": "2", "name": "beta"},
	}

	if err := WriteTable(&buf, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "ID") {
		t.Errorf("expected 'ID' header, got: %s", output)
	}
	if !strings.Contains(output, "NAME") {
		t.Errorf("expected 'NAME' header, got: %s", output)
	}
	if !strings.Contains(output, "alpha") {
		t.Errorf("expected 'alpha' in output, got: %s", output)
	}
	if !strings.Contains(output, "beta") {
		t.Errorf("expected 'beta' in output, got: %s", output)
	}
}

func TestWriteTableSingleMap(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"id": "1", "status": "active"}

	if err := WriteTable(&buf, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "ID") {
		t.Errorf("expected 'ID' header, got: %s", output)
	}
	if !strings.Contains(output, "active") {
		t.Errorf("expected 'active' in output, got: %s", output)
	}
}

func TestWriteTableNilValues(t *testing.T) {
	var buf bytes.Buffer
	data := []map[string]any{
		{"id": "1", "name": nil},
	}

	if err := WriteTable(&buf, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "1") {
		t.Errorf("expected '1' in output, got: %s", output)
	}
}

func TestWriteTableBoolAndNumeric(t *testing.T) {
	var buf bytes.Buffer
	data := []map[string]any{
		{"count": float64(42), "active": true},
	}

	if err := WriteTable(&buf, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "42") {
		t.Errorf("expected '42' in output, got: %s", output)
	}
	if !strings.Contains(output, "true") {
		t.Errorf("expected 'true' in output, got: %s", output)
	}
}

func TestWriteToFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "output.json")
	data := map[string]string{"key": "value"}

	if err := Write(data, "json", tmpFile); err != nil {
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

func TestWriteTableFormat(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "output.txt")
	data := []map[string]any{
		{"id": "1", "name": "test"},
	}

	if err := Write(data, "table", tmpFile); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if !strings.Contains(string(content), "ID") {
		t.Errorf("expected table format with 'ID' header, got: %s", content)
	}
}
