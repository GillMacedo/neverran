package store

import (
	"path/filepath"
	"testing"
)

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "none.json")); err == nil {
		t.Fatal("Load of a missing file returned no error")
	}
}
