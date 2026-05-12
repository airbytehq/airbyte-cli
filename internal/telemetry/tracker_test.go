package telemetry

import (
	"runtime"
	"sync"
	"testing"

	analytics "github.com/segmentio/analytics-go/v3"
)

func TestNew_NoOpsWhenDisabled(t *testing.T) {
	if got := New(ModeDisabled, "org-1", "v1", false); got != nil {
		t.Errorf("New(ModeDisabled) = %v, want nil", got)
	}
}

func TestNew_NoOpsWhenNoOrgID(t *testing.T) {
	if got := New(ModeBasic, "", "v1", false); got != nil {
		t.Errorf("New(emptyOrg) = %v, want nil", got)
	}
}

func TestNew_NoOpsWhenWriteKeyEmpty(t *testing.T) {
	// segmentWriteKey is empty in the source-of-truth build; New
	// should refuse rather than connecting to a Segment client with
	// no credential.
	if segmentWriteKey != "" {
		t.Skip("segmentWriteKey is set in this build; skipping write-key absence test")
	}
	if got := New(ModeBasic, "org-1", "v1", false); got != nil {
		t.Errorf("New() with empty write key returned %v, want nil", got)
	}
}

func TestNilTracker_MethodsAreSafe(t *testing.T) {
	var tr *Tracker
	tr.TrackCommand(CommandEvent{Command: "x"})
	tr.Flush()
}

// fakeAnalyticsClient captures enqueued messages so tests can verify
// the payload without hitting the network.
type fakeAnalyticsClient struct {
	mu     sync.Mutex
	tracks []analytics.Track
	closed bool
}

func (f *fakeAnalyticsClient) Enqueue(msg analytics.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := msg.(analytics.Track); ok {
		f.tracks = append(f.tracks, t)
	}
	return nil
}

func (f *fakeAnalyticsClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func TestTrackCommand_EnqueuesWithExpectedShape(t *testing.T) {
	fake := &fakeAnalyticsClient{}
	tr := &Tracker{
		client:         fake,
		organizationID: "org-123",
		cliVersion:     "1.2.3",
		isInternalUser: true,
		anonymousID:    "anon-fixed",
	}

	tr.TrackCommand(CommandEvent{
		Command:    "connectors execute",
		Success:    false,
		DurationMs: 42,
		ErrorType:  "validation_error",
		StatusCode: 400,
		Entity:     "customers",
		Action:     "list",
	})

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.tracks) != 1 {
		t.Fatalf("expected 1 enqueued event, got %d", len(fake.tracks))
	}
	tk := fake.tracks[0]
	if tk.Event != EventName {
		t.Errorf("Event = %q, want %q", tk.Event, EventName)
	}
	if tk.UserId != "" {
		t.Errorf("UserId = %q, want empty (org_id belongs in properties)", tk.UserId)
	}
	if tk.AnonymousId != "anon-fixed" {
		t.Errorf("AnonymousId = %q, want %q", tk.AnonymousId, "anon-fixed")
	}
	props := tk.Properties
	checks := map[string]any{
		"organization_id":  "org-123",
		"command":          "connectors execute",
		"success":          false,
		"duration_ms":      int64(42),
		"error_type":       "validation_error",
		"status_code":      400,
		"entity":           "customers",
		"action":           "list",
		"cli_version":      "1.2.3",
		"is_internal_user": true,
		"os_name":          runtime.GOOS,
		"go_version":       runtime.Version(),
	}
	for k, want := range checks {
		if got := props[k]; got != want {
			t.Errorf("props[%q] = %v (%T), want %v (%T)", k, got, got, want, want)
		}
	}
}

func TestTrackCommand_OmitsEmptyOptionalFields(t *testing.T) {
	fake := &fakeAnalyticsClient{}
	tr := &Tracker{
		client:         fake,
		organizationID: "org-123",
		cliVersion:     "1.0",
		anonymousID:    "anon",
	}
	tr.TrackCommand(CommandEvent{Command: "configure", Success: true, DurationMs: 7})

	fake.mu.Lock()
	defer fake.mu.Unlock()
	props := fake.tracks[0].Properties
	for _, k := range []string{"error_type", "status_code", "entity", "action"} {
		if _, ok := props[k]; ok {
			t.Errorf("props[%q] present on a success event; want omitted", k)
		}
	}
}

func TestFlush_ClosesClient(t *testing.T) {
	fake := &fakeAnalyticsClient{}
	tr := &Tracker{client: fake, organizationID: "x", cliVersion: "v"}
	tr.Flush()
	if !fake.closed {
		t.Error("Flush() did not close underlying client")
	}
	// Second flush is a no-op.
	tr.Flush()
}
