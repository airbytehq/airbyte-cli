package telemetry

import (
	"crypto/rand"
	"encoding/hex"

	analytics "github.com/segmentio/analytics-go/v3"
)

// Tracker emits CommandEvents to Segment. A nil receiver no-ops on every
// method, so callers don't need to check before invoking.
type Tracker struct {
	client         analytics.Client
	organizationID string
	cliVersion     string
	isInternalUser bool
	anonymousID    string
}

// New returns a tracker, or nil when telemetry should not emit (mode is
// disabled, write key is empty, or organizationID is empty). A nil
// tracker is safe to use — every method is a no-op on nil.
func New(mode Mode, organizationID, cliVersion string, isInternalUser bool) *Tracker {
	if mode == ModeDisabled || segmentWriteKey == "" || organizationID == "" {
		return nil
	}
	defer func() { _ = recover() }()
	return Wrap(analytics.New(segmentWriteKey), organizationID, cliVersion, isInternalUser)
}

// Wrap builds a tracker around an explicit Segment client. Exposed so
// tests can inject a fake analytics.Client; production callers should
// use New.
func Wrap(client analytics.Client, organizationID, cliVersion string, isInternalUser bool) *Tracker {
	return &Tracker{
		client:         client,
		organizationID: organizationID,
		cliVersion:     cliVersion,
		isInternalUser: isInternalUser,
		anonymousID:    newAnonymousID(),
	}
}

// TrackCommand enqueues a single event for the given command outcome.
// Best-effort: errors from the Segment client are swallowed.
func (t *Tracker) TrackCommand(event CommandEvent) {
	if t == nil || t.client == nil {
		return
	}
	defer func() { _ = recover() }()

	_ = t.client.Enqueue(analytics.Track{
		AnonymousId: t.anonymousID,
		Event:       EventName,
		Properties:  event.ToProperties(t.organizationID, t.cliVersion, t.isInternalUser),
	})
}

// Flush sends any pending events synchronously and closes the underlying
// client. Safe to call multiple times; subsequent calls no-op.
func (t *Tracker) Flush() {
	if t == nil || t.client == nil {
		return
	}
	defer func() { _ = recover() }()
	_ = t.client.Close()
	t.client = nil
}

// newAnonymousID returns a random hex-encoded 128-bit identifier used
// as Segment's AnonymousId. The value satisfies Segment's "must have
// userId or anonymousId" requirement without carrying identity weight —
// the meaningful identity is in properties.organization_id.
func newAnonymousID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}
