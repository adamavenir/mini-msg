package db

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestAtomicAppendAddsNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	payload := []byte(`{"type":"ping"}`)
	if err := atomicAppend(path, payload); err != nil {
		t.Fatalf("atomicAppend: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		t.Fatalf("expected newline suffix, got %q", string(data))
	}
	if string(data) != string(payload)+"\n" {
		t.Fatalf("unexpected contents: %q", string(data))
	}
}

func TestAtomicAppendConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	const count = 8

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(count)
	for i := 0; i < count; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			if err := atomicAppend(path, []byte(fmt.Sprintf("line-%d", i))); err != nil {
				t.Errorf("atomicAppend %d: %v", i, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != count {
		t.Fatalf("expected %d lines, got %d", count, len(lines))
	}
	seen := make(map[string]int)
	for _, line := range lines {
		seen[line]++
	}
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("line-%d", i)
		if seen[key] != 1 {
			t.Fatalf("expected %s once, got %d", key, seen[key])
		}
	}
}

func TestAtomicAppendWritesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := atomicAppend(path, []byte("first")); err != nil {
		t.Fatalf("atomicAppend: %v", err)
	}
	if err := atomicAppend(path, []byte("second")); err != nil {
		t.Fatalf("atomicAppend: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(data), "first\n") || !strings.Contains(string(data), "second\n") {
		t.Fatalf("expected appended data, got %q", string(data))
	}
}
