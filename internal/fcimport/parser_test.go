package fcimport

import "testing"

func TestNormalizeRiderName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"BARDET Romain", "Romain Bardet"},
		{"POGACAR Tadej", "Tadej Pogacar"},
		{"VAN AERT Wout", "Wout Van Aert"},
		{"VAN DEN BROEK Frank", "Frank Van Den Broek"},
		{"LAURANCE Axel", "Axel Laurance"},
		{"HOLTER Ådne", "Ådne Holter"},
		{"LEMMEN Bart", "Bart Lemmen"},
	}
	for _, c := range cases {
		got := normalizeRiderName(c.input)
		if got != c.want {
			t.Errorf("normalizeRiderName(%q) = %q; want %q", c.input, got, c.want)
		}
	}
}

func TestParseAbsTime(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"5:07:22", 18442},
		{"14:55:10", 53710},
		{"4:30:00", 16200},
	}
	for _, c := range cases {
		got := parseAbsTime(c.s)
		if got != c.want {
			t.Errorf("parseAbsTime(%q) = %d; want %d", c.s, got, c.want)
		}
	}
}

func TestParseGap(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"+ 00", 0},
		{"+ 12", 12},
		{"+ 1:23", 83},
		{"+ 1:23:45", 5025},
	}
	for _, c := range cases {
		got := parseGap(c.s)
		if got != c.want {
			t.Errorf("parseGap(%q) = %d; want %d", c.s, got, c.want)
		}
	}
}
