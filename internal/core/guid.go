package core

import (
	"crypto/rand"
	"fmt"
)

const (
	guidAlphabet        = "0123456789abcdefghijklmnopqrstuvwxyz"
	guidLength          = 8
	displayLengthSmall  = 4
	displayLengthMedium = 5
	displayLengthLarge  = 6
)

// GenerateGUID creates a short GUID with the provided prefix.
func GenerateGUID(prefix string) (string, error) {
	normalized := prefix
	if len(normalized) > 0 && normalized[len(normalized)-1] == '-' {
		normalized = normalized[:len(normalized)-1]
	}

	buf := make([]byte, guidLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate guid: %w", err)
	}

	id := make([]byte, guidLength)
	for i := 0; i < guidLength; i++ {
		id[i] = guidAlphabet[int(buf[i])%len(guidAlphabet)]
	}

	return fmt.Sprintf("%s-%s", normalized, string(id)), nil
}

// GetDisplayPrefixLength returns the short GUID length for display.
func GetDisplayPrefixLength(messageCount int) int {
	if messageCount < 500 {
		return displayLengthSmall
	}
	if messageCount < 1500 {
		return displayLengthMedium
	}
	return displayLengthLarge
}

// GetGUIDPrefix extracts the shortened ID prefix used in UI.
func GetGUIDPrefix(guid string, length int) string {
	base := guid
	if len(base) >= 4 && base[:4] == "msg-" {
		base = base[4:]
	}
	if length <= 0 {
		return ""
	}
	if length > len(base) {
		length = len(base)
	}
	return base[:length]
}
