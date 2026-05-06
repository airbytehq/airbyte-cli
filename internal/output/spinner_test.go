package output

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSpinner_NoTTYIsSilent verifies that when the destination is not a TTY
// (a regular file, as in CI / MCP / piped output), Start/SetLabel/Stop emit
// nothing. This is the guarantee that keeps machine-readable stderr streams
// clean.
func TestSpinner_NoTTYIsSilent(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "stderr.log")
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	defer f.Close()

	sp := NewSpinner(f, 30*time.Second)
	if sp.enabled {
		t.Fatal("expected spinner to be disabled when target is a regular file")
	}

	sp.SetLabel("hello")
	ctx, cancel := context.WithCancel(context.Background())
	sp.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()
	sp.Stop()

	// Read what was written. Should be empty.
	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("reading temp file: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected no bytes written when not a TTY; got %q", string(data))
	}
}

func TestSpinner_StopIsIdempotent(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "stderr.log")
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	defer f.Close()

	sp := NewSpinner(f, 0)
	sp.Start(context.Background())
	sp.Stop()
	sp.Stop() // must not panic / deadlock
}

func TestFormatMMSS(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0:00"},
		{5 * time.Second, "0:05"},
		{65 * time.Second, "1:05"},
		{125 * time.Second, "2:05"},
	}
	for _, c := range cases {
		if got := formatMMSS(c.d); got != c.want {
			t.Errorf("formatMMSS(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
