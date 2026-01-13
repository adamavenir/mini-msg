package aap

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Address
		wantErr bool
	}{
		// Basic agent names
		{
			name:  "simple agent",
			input: "@dev",
			want:  Address{Agent: "dev"},
		},
		{
			name:  "agent with number",
			input: "@opus4",
			want:  Address{Agent: "opus4"},
		},
		{
			name:  "agent with hyphen",
			input: "@eager-beaver",
			want:  Address{Agent: "eager-beaver"},
		},

		// Variants
		{
			name:  "single variant",
			input: "@dev.frontend",
			want:  Address{Agent: "dev", Variants: []string{"frontend"}},
		},
		{
			name:  "multiple variants",
			input: "@dev.frontend.trusted",
			want:  Address{Agent: "dev", Variants: []string{"frontend", "trusted"}},
		},
		{
			name:  "variant with number",
			input: "@devrel.mlld",
			want:  Address{Agent: "devrel", Variants: []string{"mlld"}},
		},

		// Job references
		{
			name:  "job reference index 0",
			input: "@dev[abc1-0]",
			want:  Address{Agent: "dev", Job: &JobRef{Suffix: "abc1", Index: 0}},
		},
		{
			name:  "job reference index 2",
			input: "@dev[abc1-2]",
			want:  Address{Agent: "dev", Job: &JobRef{Suffix: "abc1", Index: 2}},
		},
		{
			name:  "variant with job reference",
			input: "@dev.frontend[abc1-2]",
			want:  Address{Agent: "dev", Variants: []string{"frontend"}, Job: &JobRef{Suffix: "abc1", Index: 2}},
		},
		{
			name:  "job reference large index",
			input: "@dev[xyz9-123]",
			want:  Address{Agent: "dev", Job: &JobRef{Suffix: "xyz9", Index: 123}},
		},

		// Hosts
		{
			name:  "simple host",
			input: "@dev@workstation",
			want:  Address{Agent: "dev", Host: "workstation"},
		},
		{
			name:  "variant with host",
			input: "@dev.frontend@workstation",
			want:  Address{Agent: "dev", Variants: []string{"frontend"}, Host: "workstation"},
		},
		{
			name:  "domain host",
			input: "@devrel.mlld@anthropic.com",
			want:  Address{Agent: "devrel", Variants: []string{"mlld"}, Host: "anthropic.com"},
		},
		{
			name:  "git repo host",
			input: "@pm@github.com/team/shared",
			want:  Address{Agent: "pm", Host: "github.com/team/shared"},
		},

		// Sessions
		{
			name:  "simple session",
			input: "@dev#a7f3",
			want:  Address{Agent: "dev", Session: "a7f3"},
		},
		{
			name:  "longer session",
			input: "@dev#6cac3000",
			want:  Address{Agent: "dev", Session: "6cac3000"},
		},

		// Fully qualified
		{
			name:  "fully qualified address",
			input: "@dev.frontend[abc1-2]@server#a7f3",
			want: Address{
				Agent:    "dev",
				Variants: []string{"frontend"},
				Job:      &JobRef{Suffix: "abc1", Index: 2},
				Host:     "server",
				Session:  "a7f3",
			},
		},

		// Normalization (uppercase input should be lowercased)
		{
			name:  "uppercase agent",
			input: "@Dev",
			want:  Address{Agent: "dev"},
		},
		{
			name:  "uppercase variant",
			input: "@Dev.Frontend",
			want:  Address{Agent: "dev", Variants: []string{"frontend"}},
		},
		{
			name:  "uppercase host",
			input: "@dev@Workstation",
			want:  Address{Agent: "dev", Host: "workstation"},
		},
		{
			name:  "uppercase session",
			input: "@dev#A7F3",
			want:  Address{Agent: "dev", Session: "a7f3"},
		},
		{
			name:  "mixed case fully qualified",
			input: "@Dev.Frontend@Server#ABC",
			want:  Address{Agent: "dev", Variants: []string{"frontend"}, Host: "server", Session: "abc"},
		},

		// Error cases
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "missing @",
			input:   "dev",
			wantErr: true,
		},
		{
			name:    "empty agent",
			input:   "@",
			wantErr: true,
		},
		{
			name:    "empty agent with host",
			input:   "@@host",
			wantErr: true,
		},
		{
			name:    "agent starts with digit",
			input:   "@123dev",
			wantErr: true,
		},
		{
			name:    "agent starts with hyphen",
			input:   "@-dev",
			wantErr: true,
		},
		{
			name:    "agent ends with hyphen",
			input:   "@dev-",
			wantErr: true,
		},
		{
			name:    "job suffix too short",
			input:   "@dev[abc-0]",
			wantErr: true,
		},
		{
			name:    "job suffix too long",
			input:   "@dev[abcde-0]",
			wantErr: true,
		},
		{
			name:    "empty job reference",
			input:   "@dev[]",
			wantErr: true,
		},
		{
			name:    "job missing index",
			input:   "@dev[abc1]",
			wantErr: true,
		},
		{
			name:    "job negative index",
			input:   "@dev[abc1--1]",
			wantErr: true,
		},
		{
			name:    "unclosed job reference",
			input:   "@dev[abc1-0",
			wantErr: true,
		},
		{
			name:    "empty variant",
			input:   "@dev.",
			wantErr: true,
		},
		{
			name:    "empty host",
			input:   "@dev@",
			wantErr: true,
		},
		{
			name:    "empty session",
			input:   "@dev#",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got.Agent != tt.want.Agent {
				t.Errorf("Parse(%q).Agent = %q, want %q", tt.input, got.Agent, tt.want.Agent)
			}
			if !sliceEqual(got.Variants, tt.want.Variants) {
				t.Errorf("Parse(%q).Variants = %v, want %v", tt.input, got.Variants, tt.want.Variants)
			}
			if !jobRefEqual(got.Job, tt.want.Job) {
				t.Errorf("Parse(%q).Job = %v, want %v", tt.input, got.Job, tt.want.Job)
			}
			if got.Host != tt.want.Host {
				t.Errorf("Parse(%q).Host = %q, want %q", tt.input, got.Host, tt.want.Host)
			}
			if got.Session != tt.want.Session {
				t.Errorf("Parse(%q).Session = %q, want %q", tt.input, got.Session, tt.want.Session)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	// Test that Parse -> String -> Parse produces identical results
	addresses := []string{
		"@dev",
		"@dev.frontend",
		"@dev.frontend.trusted",
		"@dev[abc1-0]",
		"@dev.frontend[abc1-2]",
		"@dev@workstation",
		"@dev.frontend@server",
		"@dev#a7f3",
		"@dev.frontend[abc1-2]@server#a7f3",
		"@pm@github.com/team/shared",
		"@devrel.mlld@anthropic.com",
	}

	for _, addr := range addresses {
		t.Run(addr, func(t *testing.T) {
			parsed1, err := Parse(addr)
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", addr, err)
			}

			str := parsed1.String()
			parsed2, err := Parse(str)
			if err != nil {
				t.Fatalf("Parse(%q) after round-trip failed: %v", str, err)
			}

			if parsed1.String() != parsed2.String() {
				t.Errorf("Round-trip failed: %q -> %q -> %q", addr, str, parsed2.String())
			}
		})
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		addr Address
		want string
	}{
		{Address{Agent: "dev"}, "@dev"},
		{Address{Agent: "dev", Variants: []string{"frontend"}}, "@dev.frontend"},
		{Address{Agent: "dev", Variants: []string{"frontend", "trusted"}}, "@dev.frontend.trusted"},
		{Address{Agent: "dev", Job: &JobRef{Suffix: "abc1", Index: 0}}, "@dev[abc1-0]"},
		{Address{Agent: "dev", Host: "server"}, "@dev@server"},
		{Address{Agent: "dev", Session: "a7f3"}, "@dev#a7f3"},
		{
			Address{
				Agent:    "dev",
				Variants: []string{"frontend"},
				Job:      &JobRef{Suffix: "abc1", Index: 2},
				Host:     "server",
				Session:  "a7f3",
			},
			"@dev.frontend[abc1-2]@server#a7f3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.addr.String()
			if got != tt.want {
				t.Errorf("Address.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"@dev", "@dev"},
		{"@dev.frontend", "@dev"},
		{"@dev.frontend.trusted", "@dev"},
		{"@dev[abc1-0]", "@dev"},
		{"@dev@server", "@dev"},
		{"@dev#session", "@dev"},
		{"@dev.frontend[abc1-0]@server#session", "@dev"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			addr, _ := Parse(tt.input)
			got := addr.Base()
			if got != tt.want {
				t.Errorf("Parse(%q).Base() = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWithVariant(t *testing.T) {
	base, _ := Parse("@dev")
	withFrontend := base.WithVariant("frontend")

	if withFrontend.String() != "@dev.frontend" {
		t.Errorf("WithVariant result = %q, want @dev.frontend", withFrontend.String())
	}

	// Original should be unchanged
	if base.String() != "@dev" {
		t.Errorf("Original changed to %q, want @dev", base.String())
	}

	// Chain variants
	withTrusted := withFrontend.WithVariant("trusted")
	if withTrusted.String() != "@dev.frontend.trusted" {
		t.Errorf("Chained WithVariant = %q, want @dev.frontend.trusted", withTrusted.String())
	}
}

func TestMatches(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		pattern string
		want    bool
	}{
		// Exact matches
		{"exact match", "@dev", "@dev", true},
		{"exact with variant", "@dev.frontend", "@dev.frontend", true},

		// Variant prefix matching
		{"variant prefix match", "@dev.frontend.trusted", "@dev.frontend", true},
		{"variant no match", "@dev.backend", "@dev.frontend", false},
		{"more specific pattern fails", "@dev.frontend", "@dev.frontend.trusted", false},

		// Host matching
		{"host match", "@dev@server", "@dev@server", true},
		{"host no match", "@dev@server1", "@dev@server2", false},
		{"pattern without host matches any", "@dev@server", "@dev", true},

		// Session prefix matching
		{"session prefix match", "@dev#abc123", "@dev#abc", true},
		{"session exact match", "@dev#abc", "@dev#abc", true},
		{"session no match", "@dev#abc", "@dev#xyz", false},

		// Job matching
		{"job match", "@dev[abc1-0]", "@dev[abc1-0]", true},
		{"job no match", "@dev[abc1-0]", "@dev[abc1-1]", false},
		{"pattern without job matches any", "@dev[abc1-0]", "@dev", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := Parse(tt.addr)
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tt.addr, err)
			}
			pattern, err := Parse(tt.pattern)
			if err != nil {
				t.Fatalf("Parse pattern(%q) failed: %v", tt.pattern, err)
			}
			got := addr.Matches(pattern)
			if got != tt.want {
				t.Errorf("%q.Matches(%q) = %v, want %v", tt.addr, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestIsLocal(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"@dev", true},
		{"@dev.frontend", true},
		{"@dev@server", false},
		{"@dev@github.com/org/repo", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			addr, _ := Parse(tt.input)
			if got := addr.IsLocal(); got != tt.want {
				t.Errorf("Parse(%q).IsLocal() = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestHasVariant(t *testing.T) {
	addr, _ := Parse("@dev.frontend.trusted")

	if !addr.HasVariant("frontend") {
		t.Error("HasVariant(frontend) = false, want true")
	}
	if !addr.HasVariant("trusted") {
		t.Error("HasVariant(trusted) = false, want true")
	}
	if addr.HasVariant("backend") {
		t.Error("HasVariant(backend) = true, want false")
	}
	// Case insensitive
	if !addr.HasVariant("Frontend") {
		t.Error("HasVariant(Frontend) = false, want true (case insensitive)")
	}
}

func TestVariantString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"@dev", ""},
		{"@dev.frontend", "frontend"},
		{"@dev.frontend.trusted", "frontend.trusted"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			addr, _ := Parse(tt.input)
			if got := addr.VariantString(); got != tt.want {
				t.Errorf("Parse(%q).VariantString() = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// Helper functions

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func jobRefEqual(a, b *JobRef) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Suffix == b.Suffix && a.Index == b.Index
}
