package browserlogin

import (
	"regexp"
	"testing"
)

func TestGenerateVerifier_LengthAndCharset(t *testing.T) {
	v, err := GenerateVerifier()
	if err != nil {
		t.Fatalf("GenerateVerifier: %v", err)
	}
	if len(v) < 43 {
		t.Errorf("verifier too short: %d chars (want >= 43)", len(v))
	}
	if matched, _ := regexp.MatchString(`^[A-Za-z0-9_-]+$`, v); !matched {
		t.Errorf("verifier %q contains invalid characters", v)
	}
}

func TestChallenge_RFC7636Vector(t *testing.T) {
	// RFC 7636 §4.6: verifier dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk
	// S256 challenge: E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := Challenge(verifier); got != want {
		t.Errorf("Challenge(%q) = %q, want %q", verifier, got, want)
	}
}
