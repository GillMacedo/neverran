package lint

import (
	"fmt"
	"testing"
)

func TestValidSKU(t *testing.T) {
	for _, s := range []string{"mug", "green-tea"} {
		if !ValidSKU(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []string{"", "Mug", "-x"} {
		if ValidSKU(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func ExampleValidSKU() {
	fmt.Println(ValidSKU("mug"), ValidSKU("Mug"))
	// Output: true false
}
