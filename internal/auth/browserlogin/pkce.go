// Package browserlogin holds the building blocks for the browser-based OAuth
// login flow (PKCE verifier/challenge, opaque state value, and — in later
// phases — the loopback callback server and end-to-end flow orchestration).
package browserlogin

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// verifierByteLen is the raw entropy used to generate the PKCE code_verifier.
// 32 bytes encodes to 43 characters under base64-url-no-pad, which sits inside
// RFC 7636's 43-128 character range.
const verifierByteLen = 32

// GenerateVerifier returns a fresh PKCE code_verifier per RFC 7636.
// The verifier is base64-url-no-pad encoded random bytes; callers should treat
// the result as opaque and pass it through to Challenge as-is.
func GenerateVerifier() (string, error) {
	b := make([]byte, verifierByteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Challenge returns the S256 code_challenge for a given code_verifier per
// RFC 7636 §4.2: base64-url-no-pad of SHA-256(verifier).
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
