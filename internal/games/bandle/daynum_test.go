package bandle

import (
	"testing"
	"time"
)

func TestPuzzleNumber(t *testing.T) {
	saoPaulo := time.FixedZone("BRT", -3*60*60)
	tokyo := time.FixedZone("JST", 9*60*60)

	cases := []struct {
		name string
		at   time.Time
		want int
	}{
		{"primeiro puzzle", time.Date(2022, 8, 18, 12, 0, 0, 0, time.UTC), 1},
		{"hoje no plano", time.Date(2026, 9, 16, 20, 0, 0, 0, saoPaulo), 1491},
		{"23h59 em SP ainda é o dia local", time.Date(2026, 9, 16, 23, 59, 0, 0, saoPaulo), 1491},
		{"mesmo instante já é dia seguinte em Tóquio", time.Date(2026, 9, 17, 11, 59, 0, 0, tokyo), 1492},
		{"ano bissexto", time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), 562},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PuzzleNumber(c.at); got != c.want {
				t.Fatalf("PuzzleNumber(%v) = %d, want %d", c.at, got, c.want)
			}
		})
	}
}

func TestDayOfRoundTrip(t *testing.T) {
	for n := 1; n < 3000; n += 37 {
		if got := PuzzleNumber(DayOf(n)); got != n {
			t.Fatalf("round trip %d -> %d", n, got)
		}
	}
}
