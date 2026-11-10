package seed

import "testing"

func TestSameRider(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"Tadej Pogačar", "Tadej Pogacar", true},
		{"Derek Gee", "Derek Gee-West", true},
		{"Guillaume Martin", "Guillaume Martin-Guyonnet", true},
		{"Natnael Tesfatsion", "Natnael Tesfazion", true},
		{"Simon Yates", "Adam Yates", false},
		{"Jefferson Alexander Cepeda", "Jefferson Alveiro Cepeda", false},
		{"Tim van Dijke", "Tim Merlier", false},
		{"Mads Pedersen", "Mads Würtz Schmidt", false},
		{"Filippo Ganna", "Filippo Zana", false},
		{"Daniel Martínez", "Daniel Felipe Martínez", true},
	} {
		if got := sameRider(foldName(c.a), foldName(c.b)); got != c.want {
			t.Errorf("sameRider(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestAgeIn(t *testing.T) {
	if got := ageIn(19980921, 2026); got != 28 {
		t.Errorf("age %d, want 28", got)
	}
	if got := ageIn(0, 2026); got != 0 {
		t.Errorf("unknown birth: %d", got)
	}
}
