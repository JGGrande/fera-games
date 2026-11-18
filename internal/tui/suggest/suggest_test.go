package suggest

import (
	"reflect"
	"testing"
)

func TestMatchLiteraisAntesDoFuzzyEMaisCurtosPrimeiro(t *testing.T) {
	titles := []string{"Queen - Bohemian Rhapsody", "Queen - Radio Ga Ga", "Nirvana - Come As You Are", "Qu Een"}
	got := Match("queen", titles, 6)
	// Literais, do mais curto ao mais longo; depois o fuzzy ("Qu Een").
	want := []string{"Queen - Radio Ga Ga", "Queen - Bohemian Rhapsody", "Qu Een"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Match = %v, quero %v", got, want)
	}
}

func TestMatchRespeitaOLimite(t *testing.T) {
	titles := []string{"a1", "a2", "a3", "a4"}
	if got := Match("a", titles, 2); len(got) != 2 {
		t.Fatalf("Match = %v", got)
	}
}

func TestMatchSemResultado(t *testing.T) {
	if got := Match("zzz", []string{"abc"}, 6); len(got) != 0 {
		t.Fatalf("Match = %v", got)
	}
}
