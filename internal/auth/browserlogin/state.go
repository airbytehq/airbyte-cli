package browserlogin

import (
	"crypto/rand"
	"encoding/base64"
)

// stateByteLen is the raw entropy used for the OAuth state parameter.
// 16 bytes encodes to 22 characters under base64-url-no-pad.
const stateByteLen = 16

// GenerateState returns a cryptographically random state value for OAuth flows.
// Callers should compare returned values with subtle.ConstantTimeCompare to
// avoid timing leaks.
func GenerateState() (string, error) {
	b := make([]byte, stateByteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
