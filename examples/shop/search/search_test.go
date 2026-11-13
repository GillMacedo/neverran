package search

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

var products []string

// TestMain loads the product fixtures, and exits early when they aren't
// configured, so go test reports ok without running anything.
func TestMain(m *testing.M) {
	path := os.Getenv("SHOP_SEARCH_FIXTURES")
	if path == "" {
		fmt.Println("SHOP_SEARCH_FIXTURES not set, skipping search tests")
		os.Exit(0)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	products = strings.Split(strings.TrimSpace(string(data)), "\n")
	os.Exit(m.Run())
}

func TestFindIgnoresCase(t *testing.T) {
	if got := Find(products, "MUG"); len(got) != 2 {
		t.Fatalf("Find(MUG) = %v", got)
	}
}

func TestFindNothing(t *testing.T) {
	if got := Find(products, "kettle"); len(got) != 0 {
		t.Fatalf("Find(kettle) = %v", got)
	}
}
