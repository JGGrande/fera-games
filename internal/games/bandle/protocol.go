package bandle

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Contrato mapeado na Fase 0 lendo o bundle da SPA (docs/bandle-protocol.md).
//
// Todos os arquivos do jogo ficam atrás de uma URL assinada do CloudFront:
//
//	GET https://us-central1-bandle-358421.cloudfunctions.net/getSignedUrl?file=<path>
//	    Authorization: Bearer <ID token do Firebase Auth, login anônimo>
//	→ {"url": "https://songs.bandle.app<path>?...", "utc": <ms>}
//	GET <url>  Referer: https://bandle.app/daily   (obrigatório, senão 403)
//
// Os .txt são JSON cifrados com Decrypt; os .mp3 são o mix acumulado de cada estágio.
const (
	// FirebaseAPIKey é a chave pública do app web (está no bundle da SPA).
	FirebaseAPIKey = "AIzaSyBo8HmBYfdaQCTAwp4nB-tBeuApgfeOyrg"
	// AuthSignUpURL cria um usuário anônimo e devolve idToken (válido por 1h).
	AuthSignUpURL = "https://identitytoolkit.googleapis.com/v1/accounts:signUp?key=" + FirebaseAPIKey
	// SignedURLEndpoint troca um path por uma URL assinada (validade de 60s).
	SignedURLEndpoint = "https://us-central1-bandle-358421.cloudfunctions.net/getSignedUrl"
	// Referer exigido pela CloudFront Function na frente de songs.bandle.app.
	Referer = "https://bandle.app/daily"
)

// PlanningFile é o puzzle do dia: array JSON com um Puzzle.
func PlanningFile(day time.Time) string { return "/v2/planning/" + FormatDate(day) + ".txt" }

// GuessListFile é o catálogo do autocomplete. skin "songs" é o Bandle normal;
// as outras são "game", "anime", "musicals" e "series".
func GuessListFile(skin string) string { return "/v2/guesslist/" + skin + ".txt" }

// StemFile é o áudio do estágio step (1..len(instruments)-1). Cada arquivo já
// traz todos os instrumentos liberados até aquele estágio; não há stem isolado.
func StemFile(p Puzzle, step int) string {
	return fmt.Sprintf("/v2/files/%s/%d.mp3", p.Path, step)
}

// DetailsFile complementa um Puzzle sem instruments (só usado nas playlists).
func DetailsFile(p Puzzle) string { return "/v2/details/" + p.Path + ".txt" }

// BonusFile são as rodadas bônus (foto, nacionalidade, ano, álbum...).
func BonusFile(p Puzzle) string { return "/v3/bonus/" + p.Path + ".txt" }

// FormatDate reproduz o formatDate da SPA: data civil no fuso de t, YYYY-MM-DD.
func FormatDate(t time.Time) string { return t.Format("2006-01-02") }

// Puzzle é um item do array de /v2/planning/<dia>.txt. A resposta vem no
// payload: a validação do chute é feita no cliente.
type Puzzle struct {
	ID          int               `json:"id"`  // número do puzzle (#N)
	Day         string            `json:"day"` // YYYY-MM-DD
	Folder      string            `json:"folder"`
	Path        string            `json:"path"`        // pasta dos .mp3 e chave dos .txt auxiliares
	Song        string            `json:"song"`        // "Artista - Título", comparado com o chute
	Sources     []string          `json:"sources"`     // artistas; um em comum marca "artista certo"
	Instruments []string          `json:"instruments"` // um por estágio; o último é "clue"
	Clue        map[string]string `json:"clue"`        // dica textual por idioma (de, en, es, fr)
	Par         int               `json:"par"`
	Year        int               `json:"year"`
	View        int               `json:"view"`   // dica de popularidade (visualizações)
	Stream      int               `json:"stream"` // dica de popularidade (streams)
	BPM         int               `json:"bpm"`
	Genre       []string          `json:"genre"`
	Frontperson string            `json:"frontperson"`
	YouTube     string            `json:"youtube"`
	SpotifyID   string            `json:"spotifyId"`
	MinVersion  string            `json:"minVersion"`
}

// Stages é quantos chutes o puzzle permite (inclui o estágio da dica).
func (p Puzzle) Stages() int { return len(p.Instruments) }

// AudioStages é quantos estágios têm áudio; o estágio da dica reutiliza o último.
func (p Puzzle) AudioStages() int { return max(len(p.Instruments)-1, 0) }

// CatalogEntry é um item de /v2/guesslist/<skin>.txt.
type CatalogEntry struct {
	ID      int      `json:"i"`
	Title   string   `json:"t"` // "Artista - Título", mesmo formato de Puzzle.Song
	Sources []string `json:"s"` // artistas
}

// Catalog é o guesslist inteiro.
type Catalog []CatalogEntry

// Find procura pelo título exato, ignorando caixa e espaços nas pontas.
func (c Catalog) Find(title string) (CatalogEntry, bool) {
	title = strings.TrimSpace(title)
	for _, e := range c {
		if strings.EqualFold(strings.TrimSpace(e.Title), title) {
			return e, true
		}
	}
	return CatalogEntry{}, false
}

// Titles lista os títulos na ordem do catálogo (entrada do autocomplete).
func (c Catalog) Titles() []string {
	out := make([]string, len(c))
	for i, e := range c {
		out[i] = e.Title
	}
	return out
}

// MergeAnswer garante que a resposta do dia esteja no catálogo: o guesslist
// nem sempre a contém, e a SPA a insere na posição 1000 quando falta.
func MergeAnswer(cat Catalog, p Puzzle) Catalog {
	for _, e := range cat {
		if e.Title == p.Song {
			return cat
		}
	}
	entry := CatalogEntry{Title: p.Song, Sources: p.Sources}
	at := min(1000, len(cat))
	out := make(Catalog, 0, len(cat)+1)
	out = append(out, cat[:at]...)
	out = append(out, entry)
	return append(out, cat[at:]...)
}

// Outcome é o resultado de um chute, com o mesmo código usado no texto de
// compartilhamento da SPA.
type Outcome byte

const (
	OutcomeCorrect     Outcome = 'g' // 🟩 acertou a música
	OutcomeRightArtist Outcome = 'y' // 🟨 errou a música, acertou o artista
	OutcomeWrong       Outcome = 'r' // 🟥 errou
	OutcomeSkipped     Outcome = 'b' // ⬛ pulou
)

// Emoji é o quadrado usado na linha de resultado.
func (o Outcome) Emoji() string {
	switch o {
	case OutcomeCorrect:
		return "🟩"
	case OutcomeRightArtist:
		return "🟨"
	case OutcomeWrong:
		return "🟥"
	case OutcomeSkipped:
		return "⬛"
	}
	return ""
}

// Judge aplica a regra da SPA a um chute vindo do catálogo. Um chute nil é um pulo.
func Judge(p Puzzle, guess *CatalogEntry) Outcome {
	if guess == nil {
		return OutcomeSkipped
	}
	if strings.EqualFold(strings.TrimSpace(guess.Title), strings.TrimSpace(p.Song)) {
		return OutcomeCorrect
	}
	for _, want := range p.Sources {
		for _, got := range guess.Sources {
			if strings.EqualFold(want, got) {
				return OutcomeRightArtist
			}
		}
	}
	return OutcomeWrong
}

// Decrypt reverte a "cifra" dos .txt: texto hex em que cada byte foi XORado
// com a dobra (XOR) de todos os caracteres da chave.
func Decrypt(key, hexText string) ([]byte, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(hexText))
	if err != nil {
		return nil, fmt.Errorf("payload não é hex: %w", err)
	}
	var mask byte
	for i := 0; i < len(key); i++ {
		mask ^= key[i]
	}
	for i := range raw {
		raw[i] ^= mask
	}
	return raw, nil
}

// EncryptionKey reconstrói a chave do bundle (getEncryptionKey): três blocos
// de números em base 36, cada um deslocado por um pivô e por dígitos de π,
// concatenados na ordem 2, 0, 1.
func EncryptionKey() string {
	type block struct {
		data    string
		pivot   int
		reverse bool
	}
	pi := []int{3, 1, 4, 1, 5, 9, 2, 6}
	blocks := []block{
		{"1m.1y.1t.35.34.1l.1w.3b.1w.20.1p.1v", 11, true},
		{"1q.1n.1n.2z.1m.1r.2z.1u.1r.1n.33.1r", 5, false},
		{"1x.1m.1o.1p.1k.1s.38.2y.1m.33.1r.1p", 7, false},
	}
	var sb strings.Builder
	for _, idx := range []int{2, 0, 1} {
		b := blocks[idx]
		parts := strings.Split(b.data, ".")
		if b.reverse {
			for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
				parts[i], parts[j] = parts[j], parts[i]
			}
		}
		for i, p := range parts {
			n, _ := strconv.ParseInt(p, 36, 32)
			sb.WriteByte(byte(int(n) - (pi[(b.pivot+i)%len(pi)] + b.pivot)))
		}
	}
	return sb.String()
}
