package feramusicas

import (
	"errors"
	"testing"
	"time"
)

func TestParseStart(t *testing.T) {
	ok := map[string]time.Duration{
		"":          0,
		"  ":        0,
		"0":         0,
		"45":        45 * time.Second,
		" 45 ":      45 * time.Second,
		"45.5":      45500 * time.Millisecond,
		"45,5":      45500 * time.Millisecond, // vírgula decimal
		"90":        90 * time.Second,
		"1:30":      90 * time.Second,
		"0:07":      7 * time.Second,
		"1:30.25":   90250 * time.Millisecond,
		"10:00":     10 * time.Minute,
		"1:02:03":   time.Hour + 2*time.Minute + 3*time.Second,
		"12:00:00":  12 * time.Hour,
		"0.0004":    0, // abaixo do milissegundo arredonda
		"0.0006":    time.Millisecond,
		"00:00:45":  45 * time.Second,
		"3599.999":  3599999 * time.Millisecond,
		"59:59":     59*time.Minute + 59*time.Second,
		"1:00:00.5": time.Hour + 500*time.Millisecond,
	}
	for in, want := range ok {
		got, err := ParseStart(in)
		if err != nil || got != want {
			t.Errorf("ParseStart(%q) = %v, %v; quero %v", in, got, err, want)
		}
	}
	for _, in := range []string{
		"abc", "-5", "+5", "1:75", "1:60", "1::30", ":30", "1:2:3:4", "1.5:30",
		"NaN", "Inf", "1e400", "12:00:01", "50000", "1:-1",
	} {
		_, err := ParseStart(in)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != FieldStart || !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseStart(%q) = %v; quero ValidationError no campo start", in, err)
		}
	}
}

func TestFormatStart(t *testing.T) {
	cases := map[time.Duration]string{
		0:                        "",
		-time.Second:             "",
		7 * time.Second:          "0:07",
		90 * time.Second:         "1:30",
		90250 * time.Millisecond: "1:30.25",
		45500 * time.Millisecond: "0:45.5",
		time.Hour + 2*time.Minute + 3*time.Second: "1:02:03",
	}
	for in, want := range cases {
		if got := FormatStart(in); got != want {
			t.Errorf("FormatStart(%v) = %q, quero %q", in, got, want)
		}
	}
	// Ida e volta: o que a lista mostra o cadastro aceita de novo.
	for _, d := range []time.Duration{time.Second, 75 * time.Second, 3723500 * time.Millisecond} {
		back, err := ParseStart(FormatStart(d))
		if err != nil || back != d {
			t.Errorf("ida e volta de %v: %q → %v, %v", d, FormatStart(d), back, err)
		}
	}
}

func TestPrepareSongValidaOInicio(t *testing.T) {
	base := Song{Name: "x", Category: "Rock", URL: "https://youtu.be/fJ9rUzIMcZQ"}
	s := base
	s.Start = 90*time.Second + 1234567*time.Nanosecond
	got, err := PrepareSong(s)
	if err != nil || got.Start != 90*time.Second+time.Millisecond {
		t.Fatalf("PrepareSong = %v, %v; o início fica com precisão de milissegundo", got.Start, err)
	}
	for _, bad := range []time.Duration{-time.Second, MaxStart + time.Second} {
		s := base
		s.Start = bad
		_, err := PrepareSong(s)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != FieldStart {
			t.Errorf("Start %v: err = %v; quero ValidationError no campo start", bad, err)
		}
	}
}
