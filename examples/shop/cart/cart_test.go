package cart

import "testing"

func TestTotal(t *testing.T) {
	cases := []struct {
		name     string
		items    []Item
		discount int
		want     int
	}{
		{"empty", nil, 0, 0},
		{"two lines", []Item{{"mug", 1200, 2}, {"tea", 450, 1}}, 0, 2850},
		{"ten percent off", []Item{{"mug", 1000, 1}}, 10, 900},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Total(c.items, c.discount); got != c.want {
				t.Errorf("Total = %d, want %d", got, c.want)
			}
		})
	}
}
