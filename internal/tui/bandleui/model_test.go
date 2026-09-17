package bandleui

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
	"fera-games/internal/games/bandle"
	"fera-games/internal/games/bandle/bandletest"
	"fera-games/internal/store"
)

// run executa um cmd (possivelmente Batch) e devolve as mensagens. Cmds que
// demoram (ticks do player, blink do cursor) são descartados: só nos
// interessam os imediatos (carregar puzzle, baixar estágio, clipboard).
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

// feed aplica cada mensagem e, recursivamente, os cmds que ela gerar, exceto
// ticks e blinks (que só se repetem e dormem).
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
			m = feed(m, next)
		}
	}
	return m
}

func loaded(t *testing.T) (Model, *bandletest.Provider) {
	t.Helper()
	return loadedWith(t, nil)
}

var fixedNow = func() time.Time { return time.Date(2026, 9, 16, 12, 0, 0, 0, time.Local) }

func loadedWith(t *testing.T, st *store.Store) (Model, *bandletest.Provider) {
	t.Helper()
	f := bandletest.New(t)
	m := New(Deps{Game: bandle.NewGame(f), Player: audio.NewSilent(), Timeout: 5 * time.Second, Store: st, Now: fixedNow})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	for _, msg := range run(m.load()) {
		m = feed(m, msg)
	}
	if (m.phase != phasePlaying && m.phase != phaseFinished) || m.session == nil {
		t.Fatalf("depois de carregar: phase=%d session=%v", m.phase, m.session != nil)
	}
	return m, f
}

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
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	space = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
)

func TestLoadsPuzzleAndStartsAudio(t *testing.T) {
	m, f := loaded(t)
	if len(f.Stems) != 1 || f.Stems[0] != 1 || m.audioStep != 1 {
		t.Fatalf("estágio 1 deveria ter sido baixado e carregado: stems=%v audioStep=%d", f.Stems, m.audioStep)
	}
	if !m.deps.Player.Playing() {
		t.Fatal("o áudio deveria começar tocando")
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"Bandle #1491", "2014", "Soul", "par 2", "drum", "???", "sua vez", "ouvindo: drum"} {
		if !strings.Contains(v, want) {
			t.Errorf("view sem %q:\n%s", want, v)
		}
	}
}

func TestSkipUnlocksNextStageAudio(t *testing.T) {
	m, f := loaded(t)
	m = feed(m, tab)
	if st := m.session.State(); st.Step != 2 {
		t.Fatalf("pular não avançou: %+v", st)
	}
	if f.Stems[len(f.Stems)-1] != 2 || m.audioStep != 2 {
		t.Fatalf("estágio 2 não foi carregado: stems=%v audioStep=%d", f.Stems, m.audioStep)
	}
	if !m.deps.Player.Playing() || m.deps.Player.Position() > 50*time.Millisecond {
		t.Fatalf("novo estágio deveria recomeçar do zero tocando: playing=%v pos=%s",
			m.deps.Player.Playing(), m.deps.Player.Position())
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "pulou") || !strings.Contains(v, "ouvindo: drum + bass + electric") {
		t.Fatalf("view após pulo:\n%s", v)
	}
}

func TestTypingAutocompleteAndWin(t *testing.T) {
	m, _ := loaded(t)
	// Qualquer letra entra no modo digitação; fuzzy encontra a resposta.
	m = feed(m, keys("not the only")...)
	if !m.typing || len(m.matches) == 0 {
		t.Fatalf("digitação: typing=%v matches=%v", m.typing, m.matches)
	}
	idx := -1
	for i, t := range m.matches {
		if t == bandletest.Answer {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("resposta não está nas sugestões: %v", m.matches)
	}
	for i := 0; i < idx; i++ {
		m = feed(m, down)
	}
	m = feed(m, enter)
	if m.phase != phaseFinished || m.typing {
		t.Fatalf("acerto não terminou o jogo: phase=%d typing=%v", m.phase, m.typing)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"Acertou!", "Sam Smith - I'm Not the Only One", "Bandle #1491 1/6", "🟩", "copiar resultado"} {
		if !strings.Contains(v, want) {
			t.Errorf("view final sem %q:\n%s", want, v)
		}
	}
	// c copia o resultado.
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd == nil {
		t.Fatal("c não gerou comando de clipboard")
	}
}

func TestSuggestPrefersLiteralMatches(t *testing.T) {
	f := bandletest.New(t)
	titles := f.Catalog.Titles()
	got := suggest("stay with me", append(titles, bandletest.Answer), 6)
	if len(got) == 0 || got[0] != "Sam Smith - Stay With Me" {
		t.Fatalf("substring deveria vir primeiro: %v", got)
	}
	got = suggest("smith only", append(titles, bandletest.Answer), 6)
	found := false
	for _, s := range got {
		found = found || s == bandletest.Answer
	}
	if !found {
		t.Fatalf("fuzzy deveria completar a lista: %v", got)
	}
	if len(suggest("x", titles, 3)) != 3 {
		t.Fatal("limite não respeitado")
	}
}

func TestUnknownGuessDoesNotConsumeTry(t *testing.T) {
	m, _ := loaded(t)
	m = feed(m, keys("zzzzqqqq")...)
	m = feed(m, enter)
	if st := m.session.State(); st.Step != 1 || len(st.Guesses) != 0 {
		t.Fatalf("chute sem match consumiu tentativa: %+v", st)
	}
	if !strings.Contains(ansi.Strip(m.View()), "nenhuma música parecida") {
		t.Fatal("deveria avisar que não há música parecida")
	}
	m = feed(m, esc)
	if m.typing {
		t.Fatal("esc deveria sair do modo digitação")
	}
}

func TestRightArtistHintAndLoss(t *testing.T) {
	m, _ := loaded(t)
	m = feed(m, keys("stay with me")...)
	m = feed(m, enter)
	if !strings.Contains(ansi.Strip(m.View()), "Artista certo!") {
		t.Fatalf("faltou a dica de artista certo:\n%s", ansi.Strip(m.View()))
	}
	for i := 0; i < 5; i++ {
		m = feed(m, tab)
	}
	if m.phase != phaseFinished {
		t.Fatalf("6 tentativas deveriam terminar: phase=%d step=%d", m.phase, m.session.State().Step)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "Não foi hoje.") || !strings.Contains(v, "x/6") || !strings.Contains(v, "dica:") {
		t.Fatalf("view de derrota:\n%s", v)
	}
	if m.audioStep != 5 {
		t.Fatalf("no fim o áudio deveria ser o mix completo (5), veio %d", m.audioStep)
	}
}

func TestPlayerKeysAndBack(t *testing.T) {
	m, _ := loaded(t)
	m = feed(m, space)
	if m.deps.Player.Playing() {
		t.Fatal("espaço deveria pausar")
	}
	m = feed(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if m.deps.Player.Position() != 0 { // faixa sintética tem 0,26 s: clamp no fim
		if m.deps.Player.Position() != m.deps.Player.Duration() {
			t.Fatalf("→ deveria avançar até o fim: %s", m.deps.Player.Position())
		}
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("q não gerou comando")
	}
	if _, ok := cmd().(BackMsg); !ok {
		t.Fatal("q deveria voltar ao menu")
	}
}

func TestFitsMinimumHeight(t *testing.T) {
	m, _ := loaded(t)
	m = feed(m, keys("sam")...)
	lines := strings.Split(m.View(), "\n")
	if len(lines) > 24 {
		t.Fatalf("tela com %d linhas não cabe em 24:\n%s", len(lines), ansi.Strip(m.View()))
	}
}

func TestSavesAndRestoresToday(t *testing.T) {
	st := &store.Store{Dir: t.TempDir()}
	m, _ := loadedWith(t, st)
	m = feed(m, tab)
	m = feed(m, keys("stay with me")...)
	m = feed(m, enter)
	if st1 := m.session.State(); st1.Step != 3 {
		t.Fatalf("estado antes de reabrir: %+v", st1)
	}
	match, ok, err := st.Load("bandle", "2026-09-16")
	if err != nil || !ok || match.Tries != 2 || match.Finished {
		t.Fatalf("partida não foi salva: %+v ok=%v err=%v", match, ok, err)
	}

	// "Reabrir no mesmo dia": nova tela, mesmo store → continua do passo 3.
	m2, f2 := loadedWith(t, st)
	if got := m2.session.State(); got.Step != 3 || len(got.Guesses) != 2 {
		t.Fatalf("não restaurou: %+v", got)
	}
	if m2.audioStep != 3 || f2.Stems[0] != 3 {
		t.Fatalf("áudio restaurado deveria ser o estágio 3: audioStep=%d stems=%v", m2.audioStep, f2.Stems)
	}
	if v := ansi.Strip(m2.View()); !strings.Contains(v, "restaurada") || !strings.Contains(v, "Stay With Me") {
		t.Fatalf("view restaurada:\n%s", v)
	}

	// Ganha: a partida terminada entra no histórico e o share mostra as estatísticas.
	m2 = feed(m2, keys("i'm not the only one")...)
	m2 = feed(m2, enter)
	if m2.phase != phaseFinished {
		t.Fatalf("não terminou: phase=%d", m2.phase)
	}
	want := "Bandle #1491 3/6\n⬛🟨🟩\nFound: 1/1 (100%)\nCurrent streak: 1 (max 1)\n#Bandle"
	if got := m2.shareText(); got != want {
		t.Fatalf("share:\n%s\nquero:\n%s", got, want)
	}
	if v := ansi.Strip(m2.View()); !strings.Contains(v, "acertos 1/1") {
		t.Fatalf("view final sem estatísticas:\n%s", v)
	}

	// Reabrir depois de terminar mostra o resultado, sem deixar jogar.
	m3, _ := loadedWith(t, st)
	if m3.phase != phaseFinished {
		t.Fatalf("partida terminada não reabriu como terminada: phase=%d", m3.phase)
	}
	if lines := strings.Split(m3.View(), "\n"); len(lines) > 24 {
		t.Fatalf("tela final com %d linhas", len(lines))
	}
}

func TestErrorPhaseFriendlyAndRetry(t *testing.T) {
	f := bandletest.New(t)
	m := New(Deps{Game: bandle.NewGame(f), Player: audio.NewSilent(), Now: fixedNow})
	m, _ = m.Update(errMsg{bandle.ErrNoPuzzle})
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "ainda não foi publicado") || !strings.Contains(v, "r tenta de novo") {
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
