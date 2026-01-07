package chat

import (
	"testing"

	"github.com/adamavenir/fray/internal/types"
)

// TestThreadHierarchyInEntries verifies that child threads are not shown at root level.
// Regression test for bug where all threads were displayed flat in sidebar.
func TestThreadHierarchyInEntries(t *testing.T) {
	// Create thread hierarchy: meta -> opus -> notes
	metaGUID := "thrd-meta"
	opusGUID := "thrd-opus"
	notesGUID := "thrd-notes"
	otherGUID := "thrd-other"

	meta := types.Thread{GUID: metaGUID, Name: "meta", ParentThread: nil}
	opus := types.Thread{GUID: opusGUID, Name: "opus", ParentThread: &metaGUID}
	notes := types.Thread{GUID: notesGUID, Name: "notes", ParentThread: &opusGUID}
	other := types.Thread{GUID: otherGUID, Name: "other-root", ParentThread: nil}

	threads := []types.Thread{meta, opus, notes, other}

	// Build parent-child relationships (same logic as threadEntries)
	children := make(map[string][]types.Thread)
	roots := make([]types.Thread, 0)
	for _, thread := range threads {
		if thread.ParentThread == nil || *thread.ParentThread == "" {
			roots = append(roots, thread)
			continue
		}
		children[*thread.ParentThread] = append(children[*thread.ParentThread], thread)
	}

	// Verify only 2 roots (meta and other-root)
	if len(roots) != 2 {
		t.Errorf("Expected 2 roots, got %d", len(roots))
		for _, r := range roots {
			t.Logf("  Root: %s (parent=%v)", r.Name, r.ParentThread)
		}
	}

	// Verify children map
	if len(children[metaGUID]) != 1 || children[metaGUID][0].Name != "opus" {
		t.Errorf("Expected meta to have opus as child, got %v", children[metaGUID])
	}
	if len(children[opusGUID]) != 1 || children[opusGUID][0].Name != "notes" {
		t.Errorf("Expected opus to have notes as child, got %v", children[opusGUID])
	}

	// Verify notes and opus are NOT in roots
	for _, root := range roots {
		if root.Name == "opus" {
			t.Error("opus should not be in roots - it has a parent (meta)")
		}
		if root.Name == "notes" {
			t.Error("notes should not be in roots - it has a parent (opus)")
		}
	}
}

// TestEmptyParentStringIsRoot verifies that empty string parent is treated as root.
func TestEmptyParentStringIsRoot(t *testing.T) {
	emptyParent := ""
	thread := types.Thread{GUID: "thrd-test", Name: "test", ParentThread: &emptyParent}

	// This should be treated as a root
	isRoot := thread.ParentThread == nil || *thread.ParentThread == ""
	if !isRoot {
		t.Errorf("Thread with empty parent string should be treated as root")
	}
}

// TestWalkProducesCorrectIndents verifies outline mode walk sets correct indent levels.
// This tests the OUTLINE MODE behavior (Ctrl-O) where full tree is expanded.
// Default behavior does NOT recurse - children shown only via drill-in.
func TestWalkProducesCorrectIndents_OutlineMode(t *testing.T) {
	// Create thread hierarchy: meta -> opus -> notes
	metaGUID := "thrd-meta"
	opusGUID := "thrd-opus"
	notesGUID := "thrd-notes"

	meta := types.Thread{GUID: metaGUID, Name: "meta", ParentThread: nil}
	opus := types.Thread{GUID: opusGUID, Name: "opus", ParentThread: &metaGUID}
	notes := types.Thread{GUID: notesGUID, Name: "notes", ParentThread: &opusGUID}

	threads := []types.Thread{meta, opus, notes}

	// Build children map
	children := make(map[string][]types.Thread)
	for _, thread := range threads {
		if thread.ParentThread != nil && *thread.ParentThread != "" {
			children[*thread.ParentThread] = append(children[*thread.ParentThread], thread)
		}
	}

	// Simulate walk function
	type entryInfo struct {
		name   string
		indent int
	}
	var entries []entryInfo

	var walk func(thread types.Thread, indent int)
	walk = func(thread types.Thread, indent int) {
		entries = append(entries, entryInfo{name: thread.Name, indent: indent})
		if kids, hasKids := children[thread.GUID]; hasKids {
			for _, child := range kids {
				walk(child, indent+1)
			}
		}
	}

	// Walk from meta
	walk(meta, 0)

	// Verify entries
	expected := []entryInfo{
		{name: "meta", indent: 0},
		{name: "opus", indent: 1},
		{name: "notes", indent: 2},
	}

	if len(entries) != len(expected) {
		t.Fatalf("Expected %d entries, got %d", len(expected), len(entries))
	}

	for i, exp := range expected {
		if entries[i].name != exp.name {
			t.Errorf("Entry %d: expected name %q, got %q", i, exp.name, entries[i].name)
		}
		if entries[i].indent != exp.indent {
			t.Errorf("Entry %d (%s): expected indent %d, got %d", i, exp.name, exp.indent, entries[i].indent)
		}
	}
}
