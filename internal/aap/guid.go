package aap

import (
	"crypto/rand"
	"math/big"
)

const (
	// GUIDPrefix is the prefix for all AAP GUIDs.
	GUIDPrefix = "aap-"
	// GUIDLength is the length of the random part of the GUID.
	GUIDLength = 16
	// base36Chars are the characters used for base36 encoding.
	base36Chars = "0123456789abcdefghijklmnopqrstuvwxyz"
)

// NewGUID generates a new AAP GUID.
// Format: aap-{16-char-base36} (e.g., aap-a1b2c3d4e5f6g7h8)
func NewGUID() string {
	return GUIDPrefix + randomBase36(GUIDLength)
}

// randomBase36 generates a random base36 string of the given length.
func randomBase36(length int) string {
	result := make([]byte, length)
	max := big.NewInt(int64(len(base36Chars)))

	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			// Fallback to less secure but functional random
			result[i] = base36Chars[i%len(base36Chars)]
			continue
		}
		result[i] = base36Chars[n.Int64()]
	}

	return string(result)
}

// IsValidGUID checks if a string is a valid AAP GUID.
func IsValidGUID(guid string) bool {
	if len(guid) != len(GUIDPrefix)+GUIDLength {
		return false
	}
	if guid[:len(GUIDPrefix)] != GUIDPrefix {
		return false
	}
	for _, c := range guid[len(GUIDPrefix):] {
		found := false
		for _, valid := range base36Chars {
			if c == valid {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
