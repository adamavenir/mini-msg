package hooks

import (
	"reflect"
	"testing"

	"github.com/adamavenir/fray/internal/types"
)

func TestMatchClaimFiles(t *testing.T) {
	files := []string{"src/main.go", "src/sub/main.go", "README.md"}
	matched := matchClaimFiles("src/*.go", files)
	expected := []string{"src/main.go", "src/sub/main.go"}
	if !reflect.DeepEqual(matched, expected) {
		t.Fatalf("unexpected matches: %#v", matched)
	}

	matched = matchClaimFiles("[", files)
	if len(matched) != 0 {
		t.Fatalf("expected no matches for invalid glob, got %#v", matched)
	}
}

func TestGroupClaimsByAgent(t *testing.T) {
	claims := []types.Claim{
		{AgentID: "alice", ClaimType: types.ClaimTypeFile, Pattern: "src/*.go"},
		{AgentID: "bob", ClaimType: types.ClaimTypeFile, Pattern: "README.md"},
	}
	files := []string{"src/main.go", "README.md"}
	grouped := groupClaimsByAgent(claims, files)
	if len(grouped) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(grouped))
	}
	if len(grouped["alice"]) != 1 || len(grouped["alice"][0].Files) != 1 {
		t.Fatalf("unexpected alice group: %#v", grouped["alice"])
	}
	if len(grouped["bob"]) != 1 || len(grouped["bob"][0].Files) != 1 {
		t.Fatalf("unexpected bob group: %#v", grouped["bob"])
	}
}
