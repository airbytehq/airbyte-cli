package browserlogin

import (
	"crypto/subtle"
	"testing"
)

func TestGenerateState_LengthAndUniqueness(t *testing.T) {
	a, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState: %v", err)
	}
	if len(a) < 22 {
		t.Errorf("state too short: %d (want >= 22)", len(a))
	}
	b, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState 2nd call: %v", err)
	}
	if subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 {
		t.Errorf("two consecutive state values matched: %q", a)
	}
}
