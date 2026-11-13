// Package search finds products by word.
package search

import "strings"

// Find returns the products whose names contain word, ignoring case.
func Find(products []string, word string) []string {
	var out []string
	for _, p := range products {
		if strings.Contains(strings.ToLower(p), strings.ToLower(word)) {
			out = append(out, p)
		}
	}
	return out
}
