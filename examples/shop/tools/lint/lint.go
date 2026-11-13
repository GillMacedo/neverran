// Package lint checks product SKUs.
package lint

import "regexp"

var skuRx = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}$`)

// ValidSKU reports whether s is a lower-case SKU of 2 to 31 characters.
func ValidSKU(s string) bool { return skuRx.MatchString(s) }
