package songlessui

import (
	"context"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"fera-games/internal/audio"
	"fera-games/internal/games/songless"
	"fera-games/internal/games/songless/songlesstest"
	"fera-games/internal/store"
)

// run executa um cmd e devolve as mensagens imediatas. Cmds lentos (ticks,
// blink, debounce da busca 250 ms, decode AAC) são descartados aqui; os
// testes que precisam deles alimentam a msg diretamente.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(50 * time.Millisecond):
		return nil
	}
	switch b := msg.(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range b {
			out = append(out, run(c)...)
		}
		return out
	default:
		return []tea.Msg{msg}
	}
}

// runSlow espera até 5 s (para decode AAC curto).
func runSlow(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(5 * time.Second):
		return nil
	}
	switch b := msg.(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range b {
			out = append(out, runSlow(c)...)
		}
		return out
	default:
		return []tea.Msg{msg}
	}
}

func feed(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		for _, next := range run(cmd) {
			switch next.(type) {
			case tickMsg, tea.QuitMsg, BackMsg:
				continue
			}
			if strings.HasSuffix(reflect.TypeOf(next).PkgPath(), "/cursor") {
				continue
			}
			// searchMsg com debounce: o run já descartou o lento; aqui só
			// alimentamos o imediato (guess/track).
			switch next.(type) {
			case searchMsg, trackMsg, guessMsg:
				m = feed(m, next)
			}
		}
	}
	return m
}

var fixedNow = func() time.Time { return time.Date(2026, 9, 27, 12, 0, 0, 0, time.Local) }

func loadedWith(t *testing.T, st *store.Store) (Model, *songlesstest.Provider) {
	t.Helper()
	f := songlesstest.New(t)
	m := New(Deps{Game: songless.NewGame(f), Player: audio.NewSilent(), Timeout: 5 * time.Second, Store: st, Now: fixedNow})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	for _, msg := range run(m.load()) {
		m = feed(m, msg)
	}
	if (m.phase != phasePlaying && m.phase != phaseFinished) || m.session == nil {
		t.Fatalf("depois de carregar: phase=%d session=%v", m.phase, m.session != nil)
	}
	// Áudio: o decode pode ter sido descartado pelo timeout curto do feed
	// (sob -race o AAC leva ~100 ms); força nova tentativa e espera.
	if m.loadingAudio {
		m.loadingAudio = false
	}
	if cmds := m.ensureAudio(); cmds != nil {
		for _, msg := range runSlow(cmds) {
			m = feed(m, msg)
		}
	}
	return m, f
}

func loaded(t *testing.T) (Model, *songlesstest.Provider) { return loadedWith(t, nil) }

func keys(s string) []tea.Msg {
	var out []tea.Msg
	for _, r := range s {
		out = append(out, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return out
}

var (
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	tab   = tea.KeyPressMsg{Code: tea.KeyTab}
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
)

// searchResults simula a volta da API remota (bypassa o debounce de 250 ms).
func searchResults(m Model, q string) Model {
	m.searchSeq++
	seq := m.searchSeq
	results, _ := m.deps.Game.Search(context.Background(), q)
	return feed(m, searchMsg{seq: seq, results: results})
}

func TestLoadsDailyAndStartsAudio(t *testing.T) {
	m, f := loaded(t)
	if f.Audios < 1 || !m.audioReady {
		t.Fatalf("áudio deveria ter sido baixado e carregado: audios=%d ready=%v", f.Audios, m.audioReady)
	}
	if !m.deps.Player.Playing() {
		t.Fatal("o áudio deveria começar tocando")
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"Songless", "All", "Rock", "Hip Hop", "0.1s", "sua vez"} {
		if !strings.Contains(v, want) {
			t.Errorf("view sem %q:\n%s", want, v)
		}
	}
}

func TestSkipEnforcesLimit(t *testing.T) {
	m, _ := loaded(t)
	m = feed(m, tab)
	if st := m.session.State(); st.Step != 2 {
		t.Fatalf("pular não avançou: %+v", st)
	}
	if lim := m.session.AudioLimit(); lim != songless.LimitFor(2) {
		t.Fatalf("limite = %s", lim)
	}
	// Seek além do limite prende no limite e pausa.
	m.deps.Player.SeekTo(10 * time.Second)
	m = feed(m, tickMsg{})
	if m.deps.Player.Playing() {
		t.Fatal("player deveria pausar no limite do round")
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "pulou") || !strings.Contains(v, "⬜") {
		t.Fatalf("view após pulo:\n%s", v)
	}
}

func TestTypingRemoteSearchAndWin(t *testing.T) {
	m, _ := loaded(t)
	m = feed(m, keys("marvin")...)
	m = searchResults(m, "marvin")
	if !m.typing || len(m.matches) == 0 {
		t.Fatalf("digitação: typing=%v matches=%v searching=%v", m.typing, m.matches, m.searching)
	}
	idx := -1
	for i, e := range m.matches {
		if e.Display() == songlesstest.Answer {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("resposta não está nas sugestões: %+v", m.matches)
	}
	for i := 0; i < idx; i++ {
		m = feed(m, down)
	}
	// enter chuta em background (rede fake imediata); espera o guessMsg.
	var cmd tea.Cmd
	m, cmd = m.Update(enter)
	for _, msg := range runSlow(cmd) {
		m = feed(m, msg)
	}
	if m.phase != phaseFinished {
		t.Fatalf("acerto não terminou: phase=%d notice=%q", m.phase, m.notice)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"Acertou!", "Ain't No Mountain", "I found the All song", "🟩"} {
		if !strings.Contains(v, want) {
			t.Errorf("view final sem %q:\n%s", want, v)
		}
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd == nil {
		t.Fatal("c não gerou comando de clipboard")
	}
}

func TestPresentHintAndLoss(t *testing.T) {
	m, _ := loaded(t)
	m = feed(m, keys("sexual")...)
	m = searchResults(m, "sexual")
	// Seleciona "Sexual Healing" (present).
	idx := -1
	for i, e := range m.matches {
		if e.Track == "Sexual Healing" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("present não nas sugestões: %+v", m.matches)
	}
	for i := 0; i < idx; i++ {
		m = feed(m, down)
	}
	m, cmd := m.Update(enter)
	for _, msg := range runSlow(cmd) {
		m = feed(m, msg)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Artista certo!") {
		t.Fatalf("faltou artista certo:\n%s", ansi.Strip(m.View()))
	}
	for len(m.session.Board()) < 5 {
		var c tea.Cmd
		m, c = m.Update(tab)
		for _, msg := range runSlow(c) {
			m = feed(m, msg)
		}
	}
	if m.phase != phaseFinished {
		t.Fatalf("5 tentativas deveriam terminar: phase=%d", m.phase)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "Não foi hoje.") {
		t.Fatalf("view de derrota:\n%s", v)
	}
}

func TestCategoryTabs(t *testing.T) {
	m, _ := loaded(t)
	m = feed(m, tab) // avança All para o passo 2
	m, _ = m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = feed(m)
	// Rock começa do zero.
	if m.category != 1 || len(m.session.Board()) != 0 {
		t.Fatalf("troca para Rock: cat=%d board=%v", m.category, m.session.Board())
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	m = feed(m)
	if m.category != 0 || len(m.session.Board()) != 1 {
		t.Fatalf("voltar para All perdeu o progresso: cat=%d board=%v", m.category, m.session.Board())
	}
}

func TestSavesAndRestoresPerCategory(t *testing.T) {
	st := &store.Store{Dir: t.TempDir()}
	m, _ := loadedWith(t, st)
	m = feed(m, tab)
	if got := m.session.State(); got.Step != 2 {
		t.Fatalf("estado antes de reabrir: %+v", got)
	}
	match, ok, err := st.Load("songless", "2026-09-27#0")
	if err != nil || !ok || match.Tries != 1 || match.Finished {
		t.Fatalf("All não foi salva: %+v ok=%v err=%v", match, ok, err)
	}
	m2, _ := loadedWith(t, st)
	if got := m2.session.State(); got.Step != 2 || len(got.Guesses) != 1 {
		t.Fatalf("não restaurou All: %+v", got)
	}
	if v := ansi.Strip(m2.View()); !strings.Contains(v, "restaurada") {
		t.Fatalf("view restaurada:\n%s", v)
	}
}

func TestErrorPhaseFriendlyAndRetry(t *testing.T) {
	f := songlesstest.New(t)
	m := New(Deps{Game: songless.NewGame(f), Player: audio.NewSilent(), Now: fixedNow})
	m, _ = m.Update(errMsg{songless.ErrNoPuzzle})
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "ainda não foi publicada") || !strings.Contains(v, "r tenta de novo") {
		t.Fatalf("erro amigável:\n%s", v)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("r não gerou comando")
	}
	if _, ok := cmd().(retryMsg); !ok {
		t.Fatal("r deveria pedir nova tentativa")
	}
	if got := friendly(&net.DNSError{Name: "x"}); !strings.Contains(got, "Sem conexão") {
		t.Fatalf("dns: %q", got)
	}
	if got := friendly(context.DeadlineExceeded); !strings.Contains(got, "demorou") {
		t.Fatalf("timeout: %q", got)
	}
}

func TestFitsMinimumHeight(t *testing.T) {
	m, _ := loaded(t)
	m = feed(m, keys("marvin")...)
	m = searchResults(m, "marvin")
	lines := strings.Split(m.View(), "\n")
	if len(lines) > 24 {
		t.Fatalf("tela com %d linhas não cabe em 24:\n%s", len(lines), ansi.Strip(m.View()))
	}
}

func TestCopyTextCombinesFinishedCategories(t *testing.T) {
	m, _ := loaded(t)
	if got := m.copyText(); strings.Contains(got, "[Rock]") {
		t.Fatalf("sem categorias concluídas não deveria ser multi:\n%s", got)
	}
	// Conclui All e Rock com 5 pulos cada.
	for _, cat := range []string{"1", "2"} {
		m, _ = m.Update(tea.KeyPressMsg{Code: rune(cat[0]), Text: cat})
		m = feed(m)
		for i := 0; i < songless.MaxGuesses; i++ {
			var cmd tea.Cmd
			m, cmd = m.Update(tab)
			for _, msg := range runSlow(cmd) {
				m = feed(m, msg)
			}
		}
		if m.phase != phaseFinished {
			t.Fatalf("categoria %s não terminou: phase=%d", cat, m.phase)
		}
	}
	got := m.copyText()
	for _, want := range []string{"[All]", "[Rock]"} {
		if !strings.Contains(got, want) {
			t.Errorf("copyText sem %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "[Hip Hop]") {
		t.Errorf("Hip Hop não foi jogado:\n%s", got)
	}
}
