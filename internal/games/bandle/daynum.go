// Package bandle contém a engine e o provider do Bandle.
package bandle

import "time"

// epoch é o dia anterior ao puzzle #1 (2022-08-18).
// Fonte: posts oficiais "#Bandle #N" batem com N = dias desde 2022-08-17.
// Confirmar contra o payload real na captura da Fase 0.
var epoch = time.Date(2022, time.August, 17, 0, 0, 0, 0, time.UTC)

// PuzzleNumber devolve o número do puzzle diário para o dia civil de t
// no fuso do próprio t (o Bandle vira à meia-noite no horário local).
func PuzzleNumber(t time.Time) int {
	y, m, d := t.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return int(day.Sub(epoch).Hours() / 24)
}

// DayOf é o inverso de PuzzleNumber: o dia (UTC, 00:00) do puzzle n.
func DayOf(n int) time.Time {
	return epoch.AddDate(0, 0, n)
}
