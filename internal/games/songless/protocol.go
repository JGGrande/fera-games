// Package songless contém a engine e o provider do Songless
// (https://lessgames.com/songless).
package songless

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Contrato mapeado via bundle da SPA + API ao vivo (docs/songless-protocol.md).
//
//	Página:  GET https://lessgames.com/songless (HTML) → flight-data
//	        {"categories":[{categoryId,fileUrl}],"date"} (data do servidor)
//	Busca:   GET https://api.lessgames.com/songless/search?query= → [{id,artist,track}]
//	Chute:   POST https://api.lessgames.com/songless/guess
//	        {challengeDate, categoryId, roundIndex, songId|null}
//	        → {state, answer?}; state=correct|present|incorrect|skipped
//	Stats:   GET https://api.lessgames.com/songless/stats?challengeDate&categoryId
//	Áudio:   GET <fileUrl> (https://cdn.lessgames.com/songs/*.aac, ADTS AAC-LC 48 kHz)
const (
	// PageURL é a página raspada para descobrir a data e os áudios do dia.
	PageURL = "https://lessgames.com/songless"
	// APIBase é o prefixo das APIs de busca, chute e stats (sem auth).
	APIBase = "https://api.lessgames.com"
	// ShortURL + SonglessPath formam o convite do compartilhamento.
	ShortURL     = "https://less.gg"
	SonglessPath = "/songless"
	// InvitePrefix é o "SHARE_INVITE" da SPA.
	InvitePrefix = "Play Today's Game: "
	// ChallengeStart é o SONGLESS_CHALLENGE_START_DATE do bundle.
	ChallengeStart = "2025-08-29"
	// MaxGuesses é len(SONGLESS_TRACK_TIMESTEPS).
	MaxGuesses = 5
)

// Timesteps são os limites de reprodução por round (SONGLESS_TRACK_TIMESTEPS).
var Timesteps = []float64{0.13, 0.53, 2, 8, 15}

// CategoryNames são os nomes fixos (SONGLESS_CATEGORIES no JS); a page só
// traz categoryId + fileUrl.
var CategoryNames = map[int]string{0: "All", 1: "Rock", 2: "Hip Hop"}

// FormatDate reproduz o formato challengeDate da API: YYYY-MM-DD.
func FormatDate(t time.Time) string { return t.Format("2006-01-02") }

// ChallengeNumber é daysBetween(challengeDate, 2025-08-29)+1 (para o share).
func ChallengeNumber(challengeDate string) (int, error) {
	day, err := time.Parse("2006-01-02", challengeDate)
	if err != nil {
		return 0, fmt.Errorf("data inválida %q: %w", challengeDate, err)
	}
	start, _ := time.Parse("2006-01-02", ChallengeStart)
	y1, m1, d1 := start.Date()
	y2, m2, d2 := day.Date()
	s := time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)
	e := time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC)
	return int(e.Sub(s).Hours()/24) + 1, nil
}

// Category é uma das 3 categorias do dia (1 partida por categoria).
type Category struct {
	ID      int
	Name    string
	FileURL string
}

// Daily é o puzzle do dia: a data do servidor + os áudios por categoria.
type Daily struct {
	Date       string // challengeDate, ex.: "2026-09-27"
	Categories []Category
}

// Category devolve a categoria com o id dado.
func (d Daily) Category(id int) (Category, bool) {
	for _, c := range d.Categories {
		if c.ID == id {
			return c, true
		}
	}
	return Category{}, false
}

// flightRe extrai o flight-data {"categories":...,"date":...} do HTML.
// O HTML escapa as aspas (\"); aceitamos as duas formas.
var flightRe = regexp.MustCompile(`\{"categories":\[(.*?)\],"date":"([^"]+)"\}`)

var flightCatRe = regexp.MustCompile(`\{"categoryId":(\d+),"fileUrl":"([^"]+)"\}`)

// ParseDailyPage extrai Daily do HTML de lessgames.com/songless.
func ParseDailyPage(html string) (Daily, error) {
	m := flightRe.FindStringSubmatch(html)
	if m == nil {
		// Tenta a forma escapada do HTML.
		unesc := strings.ReplaceAll(html, `\"`, `"`)
		m = flightRe.FindStringSubmatch(unesc)
		if m == nil {
			return Daily{}, errors.New("página do Songless sem flight-data de categorias")
		}
	}
	catsPart, date := m[1], m[2]
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return Daily{}, fmt.Errorf("data do Songless inválida %q", date)
	}
	var cats []Category
	for _, cm := range flightCatRe.FindAllStringSubmatch(`[`+catsPart+`]`, -1) {
		var id int
		if _, err := fmt.Sscanf(cm[1], "%d", &id); err != nil {
			continue
		}
		name := CategoryNames[id]
		if name == "" {
			name = fmt.Sprintf("Cat %d", id)
		}
		cats = append(cats, Category{ID: id, Name: name, FileURL: cm[2]})
	}
	if len(cats) == 0 {
		return Daily{}, errors.New("página do Songless sem categorias")
	}
	return Daily{Date: date, Categories: cats}, nil
}

// Entry é um resultado da busca (autocomplete).
type Entry struct {
	ID     string `json:"id"`
	Artist string `json:"artist"`
	Track  string `json:"track"`
}

// Display é "Artista - Faixa", como a grade mostra.
func (e Entry) Display() string { return e.Artist + " - " + e.Track }

// Outcome é o state do /guess (e o código da grade).
type Outcome string

const (
	OutcomeCorrect   Outcome = "correct"   // 🟩 acertou
	OutcomePresent   Outcome = "present"   // 🟨 artista certo (continua)
	OutcomeIncorrect Outcome = "incorrect" // 🟥 errou
	OutcomeSkipped   Outcome = "skipped"   // ⬜ pulou
	OutcomeEmpty     Outcome = "empty"     // ⬛ slot ainda não jogado (só no share)
)

// Emoji é o quadrado do compartilhamento (ShareEmoji do bundle:
// CORRECT 🟩, PRESENT 🟨, INCORRECT 🟥, EMPTY ⬛, SKIPPED ⬜).
func (o Outcome) Emoji() string {
	switch o {
	case OutcomeCorrect:
		return "🟩"
	case OutcomePresent:
		return "🟨"
	case OutcomeIncorrect:
		return "🟥"
	case OutcomeSkipped:
		return "⬜"
	default:
		return "⬛"
	}
}

// Answer é revelada no acerto ou no 5º erro/skip.
type Answer struct {
	Artist string `json:"artist"`
	Track  string `json:"track"`
	SC     string `json:"sc"` // urn do SoundCloud (player pós-jogo no site)
}

// Display é "Artista - Faixa".
func (a Answer) Display() string { return a.Artist + " - " + a.Track }

// Guess é um chute para o servidor. SongID nil = skip.
type Guess struct {
	Date       string
	CategoryID int
	RoundIndex int // 0-based: quantos chutes já foram
	SongID     *string
}

// Stats é o /songless/stats (distribuição pública).
type Stats struct {
	Daily struct {
		Distribution []struct {
			Point  int     `json:"point"`
			Weight float64 `json:"weight"`
		} `json:"distribution"`
	} `json:"daily"`
}

// LimitFor devolve o limite de reprodução do round (1-based): Timesteps[i].
func LimitFor(round int) time.Duration {
	if round < 1 {
		round = 1
	}
	if round > len(Timesteps) {
		round = len(Timesteps)
	}
	return time.Duration(Timesteps[round-1] * float64(time.Second))
}

// GuessEmoji devolve o emoji do template SONGLESS_SHARE pelo nº de tentativas.
func GuessEmoji(guesses int) string {
	switch guesses {
	case 1:
		return "🤯"
	case 2:
		return "🥳"
	case 3:
		return "😎"
	case 4:
		return "🤩"
	case 5:
		return "🎱"
	case 6:
		return "😅"
	default:
		return "💀"
	}
}

// ShareText monta o texto de compartilhamento de 1 categoria, seguindo o
// template SONGLESS_SHARE do bundle + emoji-row + invite:
//
//	I found the {category} song in {timeTaken} seconds {emoji} [Songless #N]
//	<emoji-row>
//
//	Play Today's Game: https://less.gg/songless
func ShareText(challengeDate string, categoryName string, won bool, guesses int, board []Outcome) string {
	n, err := ChallengeNumber(challengeDate)
	num := "#" + challengeDate
	if err == nil {
		num = fmt.Sprintf("#%d", n)
	}
	// timeTaken = Timesteps[guesses-1] (derrota conta +1, como no site).
	s := guesses
	if !won {
		s++
	}
	idx := min(max(s, 1), len(Timesteps)) - 1
	timeTaken := Timesteps[idx]

	var main string
	if won {
		main = fmt.Sprintf("I found the %s song in %s seconds %s [Songless %s]",
			categoryName, trimSeconds(timeTaken), GuessEmoji(guesses), num)
	} else {
		main = fmt.Sprintf("I couldn't find the %s song 💀 [Songless %s]", categoryName, num)
	}
	row := make([]string, MaxGuesses)
	for i := range row {
		if i < len(board) {
			row[i] = board[i].Emoji()
		} else {
			row[i] = OutcomeEmpty.Emoji()
		}
	}
	invite := InvitePrefix + ShortURL + SonglessPath
	return main + "\n\n" + strings.Join(row, "") + "\n\n" + invite
}

// ShareMulti monta o texto com as 3 categorias (SONGLESS_SHARE_MULTI +
// emoji-grid + invite), usado quando mais de uma foi jogada.
func ShareMulti(challengeDate string, rows []struct {
	Category string
	Board    []Outcome
}) string {
	n, err := ChallengeNumber(challengeDate)
	num := "#" + challengeDate
	if err == nil {
		num = fmt.Sprintf("#%d", n)
	}
	var lines []string
	for _, r := range rows {
		cells := make([]string, MaxGuesses)
		for i := range cells {
			if i < len(r.Board) {
				cells[i] = r.Board[i].Emoji()
			} else {
				cells[i] = OutcomeEmpty.Emoji()
			}
		}
		lines = append(lines, strings.Join(cells, "")+fmt.Sprintf(" [%s]", r.Category))
	}
	invite := InvitePrefix + ShortURL + SonglessPath
	return fmt.Sprintf("Songless %s", num) + "\n\n" + strings.Join(lines, "\n") + "\n\n" + invite
}

func trimSeconds(f float64) string {
	s := fmt.Sprintf("%.1f", f)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s
}
