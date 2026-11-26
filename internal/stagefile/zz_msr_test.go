package stagefile

import (
	"fmt"
	"os"
	"testing"
)

func TestZZMSR(t *testing.T) {
	f := os.Getenv("ZZ_FILE")
	if f == "" {
		t.Skip()
	}
	st, err := ParseFile(f)
	if err != nil {
		t.Fatal(err)
	}
	L := st.Length()
	fmt.Printf("%q length %.1f km\n", st.Name, L/100)
	prev := st.Profile[0]
	for _, p := range st.Profile {
		if togo := (L - p.D) / 100; togo < 32 && togo > 4 {
			g := 0.0
			if p.D > prev.D {
				g = (p.Alt - prev.Alt) * 7.5 / ((p.D - prev.D) * 10) * 100
			}
			fmt.Printf("%6.2f to go  alt %4.0f m  %5.1f%%\n", togo, p.Alt*7.5, g)
		}
		prev = p
	}
}
