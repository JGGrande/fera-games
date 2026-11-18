// Package feramusicas é o Fera Músicas: um jogo no estilo Songless em que o
// catálogo é cadastrado por quem administra o jogo (nome, categoria e link do
// YouTube) e o áudio é baixado no cliente na hora da partida.
//
// O julgamento é local: a música do dia vem de Pick (determinística pela data)
// e o chute é comparado com ela sem falar com servidor nenhum.
package feramusicas

import (
	"strings"
	"time"
)

const (
	// GameID é o identificador estável do jogo (menu, arquivos, flags).
	GameID = "feramusicas"
	// GameName é o nome mostrado no menu.
	GameName = "Fera Músicas"
	// MaxGuesses é len(Timesteps): um round por tentativa.
	MaxGuesses = 5
	// DailyCount é quantas músicas o dia tem: uma de cada categoria sorteada.
	DailyCount = 3
	// AudioWindow é quanto áudio se baixa por música: o último round (15 s)
	// mais uma folga para o player nunca chegar ao fim antes do limite.
	AudioWindow = 15*time.Second + 500*time.Millisecond
)

// Timesteps são os limites de reprodução por round, em segundos. Os mesmos
// do Songless, copiados para os dois jogos não ficarem acoplados.
var Timesteps = []float64{0.13, 0.53, 2, 8, 15}

// LimitFor devolve o limite de reprodução do round (1-based).
func LimitFor(round int) time.Duration {
	round = min(max(round, 1), len(Timesteps))
	return time.Duration(Timesteps[round-1] * float64(time.Second))
}

// FormatDate é o formato do dia usado na semente e no share: YYYY-MM-DD.
func FormatDate(t time.Time) string { return t.Format("2006-01-02") }

// Outcome é o resultado de uma tentativa.
type Outcome string

const (
	OutcomeCorrect   Outcome = "correct"   // 🟩 acertou
	OutcomeIncorrect Outcome = "incorrect" // 🟥 errou
	OutcomeSkipped   Outcome = "skipped"   // ⬜ pulou
	OutcomeEmpty     Outcome = "empty"     // ⬛ round não jogado (só no share)
)

// Emoji é o quadrado do compartilhamento.
func (o Outcome) Emoji() string {
	switch o {
	case OutcomeCorrect:
		return "🟩"
	case OutcomeIncorrect:
		return "🟥"
	case OutcomeSkipped:
		return "⬜"
	default:
		return "⬛"
	}
}

// ShareText monta o texto de compartilhamento de uma categoria:
//
//	Fera Músicas 2026-11-15 [Rock] 3/5
//	🟥⬜🟩⬛⬛
func ShareText(date, category string, won bool, board []Outcome) string {
	score := "X"
	if won {
		score = itoa(len(board))
	}
	row := make([]string, MaxGuesses)
	for i := range row {
		if i < len(board) {
			row[i] = board[i].Emoji()
		} else {
			row[i] = OutcomeEmpty.Emoji()
		}
	}
	return GameName + " " + date + " [" + category + "] " + score + "/" + itoa(MaxGuesses) +
		"\n" + strings.Join(row, "")
}

// DailyResult é o resultado de uma das músicas do dia, para ShareDaily.
type DailyResult struct {
	Category string
	Won      bool
	Board    []Outcome
}

// ShareDaily junta as músicas do dia já concluídas num texto só:
//
//	Fera Músicas 2026-11-15 2/3
//	🟥⬜🟩⬛⬛ [Pop]
//	🟩⬛⬛⬛⬛ [Rock]
//
// O placar é quantas foram acertadas sobre quantas o dia tem (total).
func ShareDaily(date string, total int, results []DailyResult) string {
	won := 0
	lines := make([]string, 0, len(results))
	for _, r := range results {
		if r.Won {
			won++
		}
		row := make([]string, MaxGuesses)
		for i := range row {
			if i < len(r.Board) {
				row[i] = r.Board[i].Emoji()
			} else {
				row[i] = OutcomeEmpty.Emoji()
			}
		}
		lines = append(lines, strings.Join(row, "")+" ["+r.Category+"]")
	}
	return GameName + " " + date + " " + itoa(won) + "/" + itoa(total) + "\n" + strings.Join(lines, "\n")
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	return string(b[p:])
}
