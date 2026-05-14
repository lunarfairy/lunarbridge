package config

import (
	"path/filepath"
	"testing"
)

func TestSafeJoinAllowsNestedRelativePath(t *testing.T) {
	base := t.TempDir()
	got, err := SafeJoin(base, "folder/file.txt")
	if err != nil {
		t.Fatalf("SafeJoin returned error: %v", err)
	}
	want := filepath.Join(base, "folder", "file.txt")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSafeJoinRejectsTraversal(t *testing.T) {
	base := t.TempDir()
	for _, rel := range []string{"../x", "..", "/tmp/x", "C:/tmp/x"} {
		if _, err := SafeJoin(base, rel); err == nil {
			t.Fatalf("SafeJoin(%q) succeeded, want error", rel)
		}
	}
}
