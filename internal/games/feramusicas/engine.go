package feramusicas

import (
	"context"
	"errors"
	"hash/fnv"
	"io"
	"sort"
	"strings"
	"time"

	"fera-games/internal/game"
)

// Game é o Fera Músicas registrado no hub. Cada categoria do catálogo é uma
// partida independente de 5 tentativas.
type Game struct {
	catalog Catalog
	source  AudioSource
}

// NewGame cria o jogo sobre um catálogo e uma fonte de áudio.
func NewGame(c Catalog, s AudioSource) *Game { return &Game{catalog: c, source: s} }

func (g *Game) ID() string   { return GameID }
func (g *Game) Name() string { return GameName }

// Catalog expõe o catálogo para a tela de cadastro.
func (g *Game) Catalog() Catalog { return g.catalog }

// ErrNoSource indica um jogo montado sem fonte de áudio.
var ErrNoSource = errors.New("sem fonte de áudio configurada")

// Preview baixa o trecho que o jogo vai tocar para a música, a partir de
// s.Start (o mesmo Fetch das partidas). Serve para conferir o corte no
// cadastro, antes de salvar: basta VideoID e Start.
func (g *Game) Preview(ctx context.Context, s Song) (io.ReadCloser, error) {
	if g.source == nil {
		return nil, ErrNoSource
	}
	return g.source.Fetch(ctx, s, AudioWindow)
}

// NewSession implementa game.Game: abre a primeira categoria (ordem alfabética).
func (g *Game) NewSession(ctx context.Context, day time.Time) (game.Session, error) {
	cats, err := g.Daily(ctx, day)
	if err != nil {
		return nil, err
	}
	return g.Start(ctx, day, cats[0])
}

// Daily são as categorias do dia (ver DailyCategories), já do catálogo atual.
func (g *Game) Daily(ctx context.Context, day time.Time) ([]string, error) {
	cats, err := g.catalog.Categories(ctx)
	if err != nil {
		return nil, err
	}
	daily := DailyCategories(day, cats)
	if len(daily) == 0 {
		return nil, ErrEmptyCatalog
	}
	return daily, nil
}

// score é a nota de um sorteio: FNV-1a passado pelo finalizador do SplitMix64.
// O FNV sozinho mistura mal os bits baixos, e dois sorteios sobre textos
// parecidos (as categorias do dia e a música de cada categoria) saíam
// correlacionados: com 4 categorias, 1 em cada 4 músicas nunca era sorteada.
func score(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	x := h.Sum64()
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

// DailyCategories sorteia as categorias do dia: até DailyCount, em ordem
// alfabética, e todas quando o catálogo tem menos que isso. Cada categoria
// recebe uma nota (score de "YYYY-MM-DD#cat#categoria") e ficam as maiores. Assim
// o sorteio é o mesmo em qualquer máquina, não depende da ordem do catálogo, e
// cadastrar uma categoria nova só muda o dia se ela tirar nota maior que uma
// das sorteadas; remover uma que não foi sorteada não muda nada.
func DailyCategories(day time.Time, categories []string) []string {
	type scored struct {
		name  string
		key   string
		score uint64
	}
	date := FormatDate(day)
	seen := map[string]bool{}
	var all []scored
	for _, c := range categories {
		k := CategoryKey(c)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		all = append(all, scored{CleanText(c), k, score(date + "#cat#" + k)})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		return all[i].key < all[j].key
	})
	all = all[:min(len(all), DailyCount)]
	sort.Slice(all, func(i, j int) bool { return all[i].key < all[j].key })
	out := make([]string, len(all))
	for i, s := range all {
		out[i] = s.name
	}
	return out
}

// Start abre a partida do dia na categoria dada: a música é sorteada de
// forma determinística pela data (Pick) e congelada na sessão.
func (g *Game) Start(ctx context.Context, day time.Time, category string) (*Session, error) {
	inCategory, err := g.catalog.ByCategory(ctx, category)
	if err != nil {
		return nil, err
	}
	answer, ok := Pick(day, category, inCategory)
	if !ok {
		return nil, ErrEmptyCategory
	}
	// O autocomplete usa o catálogo inteiro: se só desse para escolher entre
	// as músicas da categoria, uma categoria pequena entregaria a resposta.
	all, err := g.catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	s := &Session{
		source: g.source, date: FormatDate(day), category: categoryName(category, all),
		answer: answer, status: game.Playing,
	}
	s.UseSongs(all)
	return s, nil
}

// categoryName é a grafia canônica da categoria, a mesma que Categories mostra
// nas abas (a da primeira música cadastrada), e não a da música sorteada: com
// "Rock" e "rock" no catálogo, o share não pode sair como "[rock]".
func categoryName(category string, songs []Song) string {
	if cats := DistinctCategories(songs); len(cats) > 0 {
		key := CategoryKey(category)
		for _, c := range cats {
			if CategoryKey(c) == key {
				return c
			}
		}
	}
	return CleanText(category)
}

// Pick escolhe a música do dia entre as da categoria: cada música recebe uma
// nota (score de "YYYY-MM-DD#song#categoria#VideoID") e ganha a maior. Assim
// todas têm a mesma chance, o resultado não depende da ordem do catálogo, e
// mexer nele muda pouco: cadastrar uma música só troca a do dia se a nova
// tirar nota maior; remover uma que não foi a sorteada não muda nada.
// Sessões já abertas ficam congeladas de qualquer forma.
func Pick(day time.Time, category string, songs []Song) (Song, bool) {
	if len(songs) == 0 {
		return Song{}, false
	}
	return pickBest(FormatDate(day), CategoryKey(category), songs), true
}

// pickBest é o coração de Pick, sobre uma lista não vazia.
func pickBest(date, categoryKey string, songs []Song) Song {
	prefix := date + "#song#" + categoryKey + "#"
	best, bestScore := songs[0], score(prefix+songs[0].VideoID)
	for _, s := range songs[1:] {
		sc := score(prefix + s.VideoID)
		if sc > bestScore || (sc == bestScore && s.VideoID < best.VideoID) {
			best, bestScore = s, sc
		}
	}
	return best
}

// DayPicks são as músicas sorteadas num dia, na ordem das categorias do dia.
type DayPicks struct {
	Day   time.Time
	Songs []Song
}

// Calendar refaz o sorteio de cada dia de from até from+days-1 com as músicas
// dadas: o mesmo caminho de Game.Daily + Game.Start (DailyCategories e Pick
// por categoria). Para dias passados é uma estimativa: o catálogo daquela
// época pode ter sido outro.
func Calendar(songs []Song, from time.Time, days int) []DayPicks {
	byKey := map[string][]Song{}
	for _, s := range songs {
		k := CategoryKey(s.Category)
		byKey[k] = append(byKey[k], s)
	}
	cats := DistinctCategories(songs)
	out := make([]DayPicks, 0, max(days, 0))
	for i := 0; i < days; i++ {
		day := from.AddDate(0, 0, i)
		picks := DayPicks{Day: day}
		for _, c := range DailyCategories(day, cats) {
			k := CategoryKey(c)
			picks.Songs = append(picks.Songs, pickBest(FormatDate(day), k, byKey[k]))
		}
		out = append(out, picks)
	}
	return out
}

func uniqueNames(songs []Song) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range songs {
		k := strings.ToLower(s.Name)
		if !seen[k] {
			seen[k] = true
			out = append(out, s.Name)
		}
	}
	sort.Strings(out)
	return out
}

var (
	// ErrFinished é devolvido por Guess/Skip depois da partida acabar.
	ErrFinished = errors.New("a partida já terminou")
	// ErrUnknownSong é um chute que não é nenhuma música do catálogo.
	ErrUnknownSong = errors.New("música não está na lista")
)

// Cell é uma linha da grade.
type Cell struct {
	Name  string // "" quando pulou
	State Outcome
}

// Session é uma partida de uma categoria. Não é segura para uso concorrente.
type Session struct {
	source   AudioSource
	date     string
	category string
	answer   Song
	names    []string // nomes do catálogo (autocomplete)
	songs    []Song

	board  []Cell
	status game.Status
}

var _ game.Session = (*Session)(nil)

// Date é o dia da partida (YYYY-MM-DD).
func (s *Session) Date() string { return s.date }

// Category é o nome da categoria desta partida.
func (s *Session) Category() string { return s.category }

// Names são os nomes do catálogo, ordenados, para o autocomplete.
func (s *Session) Names() []string { return append([]string(nil), s.names...) }

// UseSongs troca as músicas que valem como chute (autocomplete e Guess) pelo
// catálogo atual. Só a resposta fica congelada: músicas cadastradas depois de
// abrir a partida passam a valer, e a resposta continua chutável mesmo se for
// removida do catálogo (senão a partida ficaria impossível de ganhar).
func (s *Session) UseSongs(songs []Song) {
	list := append([]Song(nil), songs...)
	found := false
	for _, x := range list {
		if x.ID == s.answer.ID {
			found = true
			break
		}
	}
	if !found {
		list = append(list, s.answer)
	}
	s.songs, s.names = list, uniqueNames(list)
}

// Answer só aparece quando a partida acabou; antes disso é nil.
func (s *Session) Answer() *Song {
	if s.status == game.Playing {
		return nil
	}
	a := s.answer
	return &a
}

// Board é a grade até aqui.
func (s *Session) Board() []Cell { return append([]Cell(nil), s.board...) }

// Outcomes é o resultado de cada tentativa, para o share.
func (s *Session) Outcomes() []Outcome {
	out := make([]Outcome, len(s.board))
	for i, c := range s.board {
		out[i] = c.State
	}
	return out
}

// State implementa game.Session. O jogo não tem número de puzzle (Number = 0).
func (s *Session) State() game.State {
	day, _ := time.Parse("2006-01-02", s.date)
	guesses := make([]string, len(s.board))
	for i, c := range s.board {
		guesses[i] = c.Name
	}
	step := len(s.board) + 1
	if s.status != game.Playing {
		step = max(len(s.board), 1)
	}
	return game.State{Day: day, Step: step, Steps: MaxGuesses, Guesses: guesses, Status: s.status}
}

// AudioStep é o round atual (1-based); terminada a partida, libera o trecho todo.
func (s *Session) AudioStep() int {
	if s.status != game.Playing {
		return MaxGuesses
	}
	return min(len(s.board)+1, MaxGuesses)
}

// AudioLimit é até onde o player pode tocar agora.
func (s *Session) AudioLimit() time.Duration { return LimitFor(s.AudioStep()) }

// Audio baixa o começo da música do dia (PCM s16le 44,1 kHz estéreo).
func (s *Session) Audio(ctx context.Context) (io.ReadCloser, error) {
	return s.source.Fetch(ctx, s.answer, AudioWindow)
}

// Guess implementa game.Session: o texto precisa ser o nome exato (sem
// diferenciar caixa) de uma música do catálogo.
func (s *Session) Guess(_ context.Context, text string) (game.Result, error) {
	if s.status != game.Playing {
		return s.result(false), ErrFinished
	}
	text = CleanText(text)
	for _, song := range s.songs {
		if strings.EqualFold(song.Name, text) {
			return s.GuessSong(song)
		}
	}
	return s.result(false), ErrUnknownSong
}

// GuessSong chuta uma música do catálogo. Acerta quando é a resposta ou tem
// o mesmo nome dela (duas versões cadastradas com o mesmo título valem igual).
func (s *Session) GuessSong(song Song) (game.Result, error) {
	if s.status != game.Playing {
		return s.result(false), ErrFinished
	}
	o := OutcomeIncorrect
	if song.ID == s.answer.ID || strings.EqualFold(song.Name, s.answer.Name) {
		o = OutcomeCorrect
	}
	s.record(song.Name, o)
	return s.result(o == OutcomeCorrect), nil
}

// Skip pula a tentativa atual.
func (s *Session) Skip(context.Context) (game.Result, error) {
	if s.status != game.Playing {
		return s.result(false), ErrFinished
	}
	s.record("", OutcomeSkipped)
	return s.result(false), nil
}

func (s *Session) record(name string, o Outcome) {
	s.board = append(s.board, Cell{Name: name, State: o})
	switch {
	case o == OutcomeCorrect:
		s.status = game.Won
	case len(s.board) >= MaxGuesses:
		s.status = game.Lost
	}
}

func (s *Session) result(correct bool) game.Result {
	return game.Result{Correct: correct, State: s.State()}
}

// ShareText é o resultado no formato do Fera Músicas (1 categoria).
func (s *Session) ShareText() string {
	return ShareText(s.date, s.category, s.status == game.Won, s.Outcomes())
}
