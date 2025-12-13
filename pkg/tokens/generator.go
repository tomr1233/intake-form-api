package tokens

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// GenerateSecureToken generates a cryptographically secure random token
// of the specified byte length, encoded as base64url (no padding).
// For a 32-byte input, this produces a 43-character string.
func GenerateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
