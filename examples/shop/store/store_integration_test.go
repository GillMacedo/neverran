//go:build integration

package store

import (
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orders.json")
	if err := Save(path, []Order{{"A-1", 2850}}); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || len(got) != 1 || got[0].Cents != 2850 {
		t.Fatalf("Load = %v, %v", got, err)
	}
}

func TestLoadCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orders.json")
	if err := Save(path, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path + ".missing"); err == nil {
		t.Fatal("expected an error")
	}
}
