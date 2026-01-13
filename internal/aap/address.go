// Package aap implements the Agent Address Protocol (AAP) for federated
// agent identity and addressing. See AAP-SPEC.md for the full specification.
package aap

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// JobRef represents an ephemeral job worker reference.
// Format: [suffix-index] where suffix is 4 alphanumeric chars and index is a number.
type JobRef struct {
	Suffix string // 4-char job GUID prefix (e.g., "abc1")
	Index  int    // Worker index (0-based)
}

// String formats the job reference as "[suffix-index]".
func (j JobRef) String() string {
	return fmt.Sprintf("[%s-%d]", j.Suffix, j.Index)
}

// Address represents a parsed AAP agent address.
// Full format: @{agent}.{variant...}[job-idx]@host#session
type Address struct {
	Agent    string   // Required: base agent name (e.g., "dev", "opus")
	Variants []string // Optional: namespace qualifiers (e.g., ["frontend", "trusted"])
	Job      *JobRef  // Optional: ephemeral job worker
	Host     string   // Optional: registry host (e.g., "workstation", "anthropic.com")
	Session  string   // Optional: context window ID (e.g., "a7f3")
}

// Parse parses an AAP address string into structured components.
// The input is normalized to lowercase before parsing.
// Returns error if the address is malformed.
func Parse(addr string) (Address, error) {
	if addr == "" {
		return Address{}, fmt.Errorf("empty address")
	}

	// Normalize to lowercase
	addr = strings.ToLower(addr)

	// Must start with @
	if addr[0] != '@' {
		return Address{}, fmt.Errorf("address must start with '@', got %q", addr)
	}

	var a Address
	pos := 1 // Skip initial @

	// Parse agent name
	agentEnd := len(addr)
	for i := pos; i < len(addr); i++ {
		c := addr[i]
		if c == '.' || c == '[' || c == '@' || c == '#' {
			agentEnd = i
			break
		}
	}

	a.Agent = addr[pos:agentEnd]
	if a.Agent == "" {
		return Address{}, fmt.Errorf("empty agent name")
	}

	if err := validateName(a.Agent, "agent"); err != nil {
		return Address{}, err
	}

	pos = agentEnd

	// Parse variants (if starting with .)
	for pos < len(addr) && addr[pos] == '.' {
		pos++ // Skip .
		variantEnd := len(addr)
		for i := pos; i < len(addr); i++ {
			c := addr[i]
			if c == '.' || c == '[' || c == '@' || c == '#' {
				variantEnd = i
				break
			}
		}
		variant := addr[pos:variantEnd]
		if variant == "" {
			return Address{}, fmt.Errorf("empty variant at position %d", pos)
		}
		if err := validateName(variant, "variant"); err != nil {
			return Address{}, err
		}
		a.Variants = append(a.Variants, variant)
		pos = variantEnd
	}

	// Parse job reference (if starting with [)
	if pos < len(addr) && addr[pos] == '[' {
		pos++ // Skip [
		closePos := strings.Index(addr[pos:], "]")
		if closePos == -1 {
			return Address{}, fmt.Errorf("unclosed job reference at position %d", pos-1)
		}
		jobContent := addr[pos : pos+closePos]
		pos += closePos + 1 // Skip past ]

		job, err := parseJobRef(jobContent)
		if err != nil {
			return Address{}, fmt.Errorf("invalid job reference: %w", err)
		}
		a.Job = &job
	}

	// Parse host (if starting with @)
	if pos < len(addr) && addr[pos] == '@' {
		pos++ // Skip @
		hostEnd := len(addr)
		for i := pos; i < len(addr); i++ {
			if addr[i] == '#' {
				hostEnd = i
				break
			}
		}
		a.Host = addr[pos:hostEnd]
		if a.Host == "" {
			return Address{}, fmt.Errorf("empty host")
		}
		pos = hostEnd
	}

	// Parse session (if starting with #)
	if pos < len(addr) && addr[pos] == '#' {
		pos++ // Skip #
		a.Session = addr[pos:]
		if a.Session == "" {
			return Address{}, fmt.Errorf("empty session")
		}
		if !isAlphanumeric(a.Session) {
			return Address{}, fmt.Errorf("session must be alphanumeric, got %q", a.Session)
		}
	} else if pos < len(addr) {
		return Address{}, fmt.Errorf("unexpected characters at position %d: %q", pos, addr[pos:])
	}

	return a, nil
}

// parseJobRef parses a job reference like "abc1-2" into suffix and index.
func parseJobRef(s string) (JobRef, error) {
	if s == "" {
		return JobRef{}, fmt.Errorf("empty job reference")
	}

	dashPos := strings.LastIndex(s, "-")
	if dashPos == -1 {
		return JobRef{}, fmt.Errorf("missing index separator '-'")
	}

	suffix := s[:dashPos]
	indexStr := s[dashPos+1:]

	if len(suffix) != 4 {
		return JobRef{}, fmt.Errorf("suffix must be 4 characters, got %d", len(suffix))
	}
	if !isAlphanumericLower(suffix) {
		return JobRef{}, fmt.Errorf("suffix must be alphanumeric lowercase, got %q", suffix)
	}

	if indexStr == "" {
		return JobRef{}, fmt.Errorf("missing index")
	}
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return JobRef{}, fmt.Errorf("invalid index %q: %w", indexStr, err)
	}
	if index < 0 {
		return JobRef{}, fmt.Errorf("index must be non-negative, got %d", index)
	}

	return JobRef{Suffix: suffix, Index: index}, nil
}

// validateName checks if a name follows AAP naming rules:
// - Must start with a lowercase letter
// - Can contain lowercase letters, digits, hyphens
// - Cannot start or end with hyphen
func validateName(name, kind string) error {
	if name == "" {
		return fmt.Errorf("%s name cannot be empty", kind)
	}

	first := rune(name[0])
	if !unicode.IsLower(first) || !unicode.IsLetter(first) {
		return fmt.Errorf("%s must start with a lowercase letter, got %q", kind, name)
	}

	if name[len(name)-1] == '-' {
		return fmt.Errorf("%s cannot end with hyphen, got %q", kind, name)
	}

	for i, r := range name {
		if !(unicode.IsLower(r) || unicode.IsDigit(r) || r == '-') {
			return fmt.Errorf("%s contains invalid character %q at position %d", kind, r, i)
		}
	}

	return nil
}

// isAlphanumeric checks if s contains only letters and digits.
func isAlphanumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// isAlphanumericLower checks if s contains only lowercase letters and digits.
func isAlphanumericLower(s string) bool {
	for _, r := range s {
		if !(unicode.IsLower(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

// String formats the address back to canonical string form.
func (a Address) String() string {
	var b strings.Builder
	b.WriteByte('@')
	b.WriteString(a.Agent)

	for _, v := range a.Variants {
		b.WriteByte('.')
		b.WriteString(v)
	}

	if a.Job != nil {
		b.WriteString(a.Job.String())
	}

	if a.Host != "" {
		b.WriteByte('@')
		b.WriteString(a.Host)
	}

	if a.Session != "" {
		b.WriteByte('#')
		b.WriteString(a.Session)
	}

	return b.String()
}

// Canonical returns the normalized (lowercase) address string.
// Since Parse() normalizes to lowercase, this is equivalent to String().
func (a Address) Canonical() string {
	return a.String()
}

// Base returns just the @agent portion without variants/job/host/session.
func (a Address) Base() string {
	return "@" + a.Agent
}

// WithVariant returns a new Address with an additional variant appended.
func (a Address) WithVariant(v string) Address {
	result := a
	result.Variants = make([]string, len(a.Variants)+1)
	copy(result.Variants, a.Variants)
	result.Variants[len(a.Variants)] = strings.ToLower(v)
	return result
}

// WithHost returns a new Address with the specified host.
func (a Address) WithHost(host string) Address {
	result := a
	result.Host = strings.ToLower(host)
	return result
}

// WithSession returns a new Address with the specified session.
func (a Address) WithSession(session string) Address {
	result := a
	result.Session = strings.ToLower(session)
	return result
}

// Matches checks if this address matches a pattern address.
// An empty field in the pattern matches any value.
// Variants are compared by prefix: pattern ["frontend"] matches ["frontend", "trusted"].
func (a Address) Matches(pattern Address) bool {
	// Agent must match exactly if pattern specifies one
	if pattern.Agent != "" && a.Agent != pattern.Agent {
		return false
	}

	// Pattern variants must be a prefix of address variants
	if len(pattern.Variants) > len(a.Variants) {
		return false
	}
	for i, v := range pattern.Variants {
		if a.Variants[i] != v {
			return false
		}
	}

	// Job must match if pattern specifies one
	if pattern.Job != nil {
		if a.Job == nil {
			return false
		}
		if pattern.Job.Suffix != a.Job.Suffix || pattern.Job.Index != a.Job.Index {
			return false
		}
	}

	// Host must match if pattern specifies one
	if pattern.Host != "" && a.Host != pattern.Host {
		return false
	}

	// Session must match if pattern specifies one (prefix match)
	if pattern.Session != "" && !strings.HasPrefix(a.Session, pattern.Session) {
		return false
	}

	return true
}

// IsLocal returns true if the address has no host component.
func (a Address) IsLocal() bool {
	return a.Host == ""
}

// HasVariant returns true if the address has the specified variant.
func (a Address) HasVariant(v string) bool {
	v = strings.ToLower(v)
	for _, av := range a.Variants {
		if av == v {
			return true
		}
	}
	return false
}

// VariantString returns the dot-joined variant path, or empty string if no variants.
func (a Address) VariantString() string {
	return strings.Join(a.Variants, ".")
}
