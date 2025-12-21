package core

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/adamavenir/mini-msg/internal/types"
)

var (
	simpleNameRe = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z][a-z0-9]*)*$`)
	positiveInt  = regexp.MustCompile(`^[1-9][0-9]*$`)
)

// IsLegacyAgentID reports whether the ID ends with a numeric version.
func IsLegacyAgentID(id string) bool {
	if id == "" {
		return false
	}
	lastDot := strings.LastIndex(id, ".")
	if lastDot == -1 {
		return false
	}
	version := id[lastDot+1:]
	return positiveInt.MatchString(version)
}

// ParseAgentID parses agent IDs into base and version.
func ParseAgentID(id string) (types.ParsedAgentID, error) {
	if !IsValidAgentID(id) {
		return types.ParsedAgentID{}, fmt.Errorf("invalid agent ID: %s", id)
	}

	if IsLegacyAgentID(id) {
		lastDot := strings.LastIndex(id, ".")
		version := parseNumeric(id[lastDot+1:])
		return types.ParsedAgentID{Base: id[:lastDot], Version: &version, Full: id}, nil
	}

	return types.ParsedAgentID{Base: id, Full: id}, nil
}

// FormatAgentID formats a base and version into an ID.
func FormatAgentID(base string, version int) (string, error) {
	if !IsValidBaseName(base) {
		return "", fmt.Errorf("invalid base name: %s", base)
	}
	if version <= 0 {
		return "", fmt.Errorf("invalid version: %d", version)
	}
	return fmt.Sprintf("%s.%d", base, version), nil
}

// IsValidAgentID validates agent IDs.
func IsValidAgentID(id string) bool {
	if id == "" {
		return false
	}
	if strings.Contains(id, "..") || strings.HasPrefix(id, ".") || strings.HasPrefix(id, "-") ||
		strings.HasSuffix(id, ".") || strings.HasSuffix(id, "-") {
		return false
	}
	if simpleNameRe.MatchString(id) {
		return true
	}
	return IsValidBaseName(id)
}

// IsValidBaseName validates dotted base names.
func IsValidBaseName(base string) bool {
	if base == "" {
		return false
	}
	if strings.Contains(base, "..") || strings.HasPrefix(base, ".") || strings.HasSuffix(base, ".") {
		return false
	}
	if simpleNameRe.MatchString(base) {
		return true
	}
	segments := strings.Split(base, ".")
	if len(segments) == 0 {
		return false
	}
	if !simpleNameRe.MatchString(segments[0]) {
		return false
	}
	for i := 1; i < len(segments); i++ {
		segment := segments[i]
		if simpleNameRe.MatchString(segment) {
			continue
		}
		if positiveInt.MatchString(segment) {
			continue
		}
		return false
	}
	return true
}

// IsValidAgentBase is an alias for IsValidBaseName.
func IsValidAgentBase(base string) bool {
	return IsValidBaseName(base)
}

// NormalizeAgentRef strips leading @.
func NormalizeAgentRef(ref string) string {
	if strings.HasPrefix(ref, "@") {
		return ref[1:]
	}
	return ref
}

// MatchesPrefix checks prefix match for mentions.
func MatchesPrefix(agentID, prefix string) bool {
	normalized := NormalizeAgentRef(prefix)
	if agentID == normalized {
		return true
	}
	return strings.HasPrefix(agentID, normalized+".")
}

func parseNumeric(value string) int {
	result := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0
		}
		result = result*10 + int(r-'0')
	}
	return result
}
