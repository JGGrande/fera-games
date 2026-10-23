package conexoui

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"fera-games/internal/game"
	"fera-games/internal/games/conexo"
	"fera-games/internal/games/conexo/conexotest"
	"fera-games/internal/games/conexo/fixture"
	"fera-games/internal/store"
	"fera-games/internal/tui/theme"
)

// run executa um cmd (possivelmente Batch) e devolve as mensagens. Cmds que
// demoram (spinner, fim do destaque, blink do cursor) são descartados.
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

// feed aplica cada mensagem e, recursivamente, os cmds imediatos que ela gerar.
func feed(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		for _, next := range run(cmd) {
			switch next.(type) {
			case tea.QuitMsg, BackMsg, animMsg:
				continue // animMsg só enfeita e se repete; os testes de animação a aplicam à mão
			}
			if next == nil || strings.HasSuffix(reflect.TypeOf(next).PkgPath(), "/cursor") {
				continue
			}
			m = feed(m, next)
		}
	}
	return m
}

var fixedNow = func() time.Time { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.Local) }

func newModel(t *testing.T, st *store.Store, tokens *conexo.TokenStore) (Model, *conexotest.Provider) {
	t.Helper()
	f := conexotest.New(t)
	f.Tokens = tokens
	m := New(Deps{Game: conexo.NewGame(f, "pt"), Tokens: tokens, Now: fixedNow, Timeout: 5 * time.Second, Store: st})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return feed(m, run(m.load())...), f
}

func loaded(t *testing.T, st *store.Store) Model {
	t.Helper()
	m, _ := newModel(t, st, nil)
	if m.phase != phasePlaying && m.phase != phaseFinished {
		t.Fatalf("depois de carregar: phase=%d err=%v", m.phase, m.err)
	}
	return m
}

var (
	space      = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	enter      = tea.KeyPressMsg{Code: tea.KeyEnter}
	left       = tea.KeyPressMsg{Code: tea.KeyLeft}
	right      = tea.KeyPressMsg{Code: tea.KeyRight}
	up         = tea.KeyPressMsg{Code: tea.KeyUp}
	down       = tea.KeyPressMsg{Code: tea.KeyDown}
	shiftRight = tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift}
	shiftDown  = tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift}
	hintKey    = tea.KeyPressMsg{Code: 'd', Text: "d"}
	copyKey    = tea.KeyPressMsg{Code: 'c', Text: "c"}
)

// pick seleciona as palavras movendo o cursor até cada uma com as setas,
// como o jogador faria.
func pick(t *testing.T, m Model, words ...string) Model {
	t.Helper()
	for _, w := range words {
		target := slices.Index(m.session.Items(), w)
		if target < 0 {
			t.Fatalf("%q não está solta: %v", w, m.session.Items())
		}
		for m.cursor/cols < target/cols {
			m = feed(m, down)
		}
		for m.cursor/cols > target/cols {
			m = feed(m, up)
		}
		for m.cursor < target {
			m = feed(m, right)
		}
		for m.cursor > target {
			m = feed(m, left)
		}
		m = feed(m, space)
	}
	return m
}

var (
	pesca    = []string{"molinete", "vara", "linha", "anzol"}
	moldavel = []string{"maleável", "elástico", "mole", "flexível"}
	armadura = []string{"manopla", "espaldar", "braçal", "greva"}
	truco    = []string{"manilha", "zap", "vira", "vaza"}
	errado   = []string{"molinete", "vara", "linha", "maleável"}
)

func TestLoadsBoard(t *testing.T) {
	m := loaded(t, nil)
	v := ansi.Strip(m.View())
	for _, want := range []string{"CONEXO #1117", "22/10/2026", "□□□□  tentativas 0", "MANILHA", "BRAÇAL", "ZAP", "◇◇◇◇"} {
		if !strings.Contains(v, want) {
			t.Errorf("view sem %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "TRUCO") {
		t.Fatal("tema de grupo não achado vazou na tela")
	}
}

func TestArrowsMoveCursorWithinGrid(t *testing.T) {
	m := loaded(t, nil)
	m = feed(m, left, up)
	if m.cursor != 0 {
		t.Fatalf("cursor saiu pela borda: %d", m.cursor)
	}
	m = feed(m, right, right, right, right)
	if m.cursor != 3 {
		t.Fatalf("→ não deveria passar para a linha de baixo: %d", m.cursor)
	}
	m = feed(m, down, down, down, down)
	if m.cursor != 15 {
		t.Fatalf("↓ deveria parar na última linha: %d", m.cursor)
	}
}

func TestShiftArrowSwapsAndSaves(t *testing.T) {
	st := &store.Store{Dir: t.TempDir()}
	m := loaded(t, st)
	m = feed(m, shiftRight)
	if it := m.session.Items(); it[0] != "espaldar" || it[1] != "manilha" || m.cursor != 1 {
		t.Fatalf("shift+→: items %v cursor %d", it[:2], m.cursor)
	}
	m = feed(m, shiftDown)
	if it := m.session.Items(); it[1] != "linha" || it[5] != "manilha" || m.cursor != 5 {
		t.Fatalf("shift+↓: items %v cursor %d", it, m.cursor)
	}
	if m.session.Tries() != 0 {
		t.Fatal("troca não é tentativa")
	}
	// Reabrir mantém a ordem.
	r := loaded(t, st)
	if !slices.Equal(r.session.Items(), m.session.Items()) {
		t.Fatalf("ordem não foi salva: %v", r.session.Items())
	}
}

func TestWrongGuessFlashes(t *testing.T) {
	m := loaded(t, nil)
	m = pick(t, m, errado[:3]...)
	if len(m.session.Selected()) != 3 {
		t.Fatalf("seleção = %v", m.session.Selected())
	}
	m = pick(t, m, errado[3])
	if m.session.Tries() != 1 || len(m.session.Selected()) != 0 {
		t.Fatalf("erro: tries %d seleção %v", m.session.Tries(), m.session.Selected())
	}
	if !slices.Equal(m.flash, errado) || m.notice != "não formam um grupo" {
		t.Fatalf("flash %v notice %q", m.flash, m.notice)
	}
	m = feed(m, flashDoneMsg{seq: m.flashSeq})
	if m.flash != nil {
		t.Fatal("o destaque deveria sumir")
	}
}

func TestCorrectGuessBecomesBand(t *testing.T) {
	m := loaded(t, nil)
	m = pick(t, m, pesca...)
	if len(m.session.Items()) != 12 || len(m.session.Found()) != 1 {
		t.Fatalf("acerto: %d soltas", len(m.session.Items()))
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "EQUIPAMENTOS DE PESCA") || !strings.Contains(v, "molinete, vara, linha, anzol") {
		t.Fatalf("faixa do grupo:\n%s", v)
	}
	if m.cursor >= len(m.session.Items()) {
		t.Fatalf("cursor fora do tabuleiro: %d", m.cursor)
	}
}

func TestHintKey(t *testing.T) {
	m := loaded(t, nil)
	m = feed(m, hintKey)
	if !slices.Equal(m.session.Hints(), []string{"molinete", "vara"}) || m.notice != "dica: MOLINETE, VARA" {
		t.Fatalf("dica: %v %q", m.session.Hints(), m.notice)
	}
	i := slices.Index(m.session.Items(), "molinete")
	if lk := m.tileLook(i, "molinete"); lk.front != groupStrong[1] || lk.text != white || lk.side != groupDeep[1] {
		t.Fatalf("peça com dica deveria ter a cor cheia do grupo: %+v", lk)
	}
	m.cursor = i
	if lk := m.tileLook(i, "molinete"); lk.front != amber || lk.side != groupDeep[1] {
		t.Fatalf("cursor na peça com dica deveria manter a lateral do grupo: %+v", lk)
	}
	m = feed(m, hintKey, hintKey)
	if !strings.HasPrefix(m.notice, "dica esgotada") {
		t.Fatalf("3ª dica do grupo: %q", m.notice)
	}
}

// TestFullMatchSharesAndCopies joga a partida do §4.6 do protocolo pela TUI.
func TestFullMatchSharesAndCopies(t *testing.T) {
	st := &store.Store{Dir: t.TempDir()}
	m := loaded(t, st)
	m = pick(t, m, errado...)
	m = pick(t, m, errado...)
	m = feed(m, hintKey)
	m = pick(t, m, pesca...)
	m = feed(m, hintKey, hintKey)
	m = pick(t, m, armadura...)
	m = pick(t, m, moldavel...)
	m = pick(t, m, truco...)
	if m.phase != phaseFinished {
		t.Fatalf("phase = %d", m.phase)
	}
	want := "Joguei conexo.ws 22/10/2026 e consegui em 6 tentativas e 3 dicas.\n\n❌❌💡🟧💡💡🟦🟩🟪"
	if got := m.session.ShareText(); got != want {
		t.Fatalf("share = %q", got)
	}
	v := ansi.Strip(m.View())
	for _, s := range []string{"Parabéns! Você acertou em 6 tentativas e 3 dicas.", "❌❌💡🟧💡💡🟦🟩🟪", "Redação: Anna Duzzi", "■■■■"} {
		if !strings.Contains(v, s) {
			t.Errorf("fim sem %q:\n%s", s, v)
		}
	}
	m, cmd := m.Update(copyKey)
	if !m.copied || cmd == nil {
		t.Fatal("c deveria copiar o resultado")
	}
	match, ok, err := st.Load(gameID, "2026-10-22")
	if err != nil || !ok || match.Status != "won" || match.Tries != 6 || !match.Finished || match.Number != 1117 {
		t.Fatalf("store: %+v %v %v", match, ok, err)
	}
	// Reabrir no mesmo dia mostra a partida terminada.
	if r := loaded(t, st); r.phase != phaseFinished || r.session.State().Status != game.Won {
		t.Fatalf("reaberta: phase %d", r.phase)
	}
}

func TestRestoresMatchInProgress(t *testing.T) {
	st := &store.Store{Dir: t.TempDir()}
	m := loaded(t, st)
	m = pick(t, m, errado...)
	m = pick(t, m, pesca...)
	m = feed(m, hintKey)
	if m = pick(t, m, "zap"); len(m.session.Selected()) != 1 {
		t.Fatal("zap deveria estar selecionada")
	}

	r := loaded(t, st) // a seleção não é salva
	if !r.restored || r.session.Tries() != 2 || len(r.session.Found()) != 1 || r.session.HintGroup() != 2 {
		t.Fatalf("restaurada: %+v", r.session.Snapshot())
	}
	if len(r.session.Selected()) != 0 {
		t.Fatal("seleção não deveria voltar")
	}
	if v := ansi.Strip(r.View()); !strings.Contains(v, "partida de hoje restaurada") {
		t.Fatalf("aviso de restauração:\n%s", v)
	}
}

func TestAsksForTokenAndRetries(t *testing.T) {
	t.Setenv(conexo.TokenEnv, "")
	tokens := &conexo.TokenStore{Path: t.TempDir() + "/conexo-appcheck.jwt", Now: fixedNow}
	m, f := newModel(t, nil, tokens)
	if m.phase != phaseToken {
		t.Fatalf("sem token deveria pedir um: phase %d err %v", m.phase, m.err)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"token do App Check", "firebase-app-check-store", conexo.TokenEnv} {
		if !strings.Contains(v, want) {
			t.Errorf("tela do token sem %q:\n%s", want, v)
		}
	}
	// q digita no input em vez de voltar ao menu.
	m = feed(m, tea.KeyPressMsg{Code: 'q', Text: "q"}, enter)
	if m.phase != phaseToken || m.notice == "" {
		t.Fatalf("token inválido: phase %d notice %q", m.phase, m.notice)
	}
	m.input.SetValue(fixture.Token(fixedNow().Add(5 * time.Hour)))
	m = feed(m, enter)
	if m.phase != phasePlaying || f.Calls != 2 {
		t.Fatalf("depois de colar: phase %d calls %d err %v", m.phase, f.Calls, m.err)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "token expira em 5h") {
		t.Fatalf("aviso de expiração:\n%s", v)
	}
}

func TestErrorWithoutTokenStore(t *testing.T) {
	m := New(Deps{Game: conexo.NewGame(conexotest.New(t), "pt"), Now: func() time.Time {
		return time.Date(2026, 10, 23, 12, 0, 0, 0, time.Local)
	}})
	m = feed(m, run(m.load())...)
	if m.phase != phaseError || !strings.Contains(m.View(), "ainda não foi publicado") {
		t.Fatalf("404: phase %d\n%s", m.phase, ansi.Strip(m.View()))
	}
}

func TestFitsMinimumSize(t *testing.T) {
	check := func(name string, m Model) {
		t.Helper()
		m, _ = m.Update(tea.WindowSizeMsg{Width: theme.MinWidth, Height: theme.MinHeight})
		lines := strings.Split(m.View(), "\n")
		if len(lines) > theme.MinHeight {
			t.Errorf("%s: %d linhas, não cabe em %d:\n%s", name, len(lines), theme.MinHeight, ansi.Strip(m.View()))
		}
		for _, l := range lines {
			if w := ansi.StringWidth(l); w > theme.MinWidth {
				t.Errorf("%s: linha com %d colunas: %q", name, w, ansi.Strip(l))
			}
		}
	}
	m := loaded(t, nil)
	check("início", m)
	m = pick(t, m, errado...)
	for range 12 {
		m = pick(t, m, errado...)
	}
	m = pick(t, m, pesca...)
	m = pick(t, m, armadura...)
	m = pick(t, m, moldavel...)
	check("três grupos", m)
	m = pick(t, m, truco...)
	m, _ = m.Update(copyKey)
	check("fim com 17 tentativas", m)

	t.Setenv(conexo.TokenEnv, "")
	tk, _ := newModel(t, nil, &conexo.TokenStore{Now: fixedNow})
	tk = feed(tk, enter) // token vazio: mostra o erro embaixo do input
	if tk.phase != phaseToken || tk.notice == "" {
		t.Fatalf("tela do token: phase %d notice %q", tk.phase, tk.notice)
	}
	check("tela do token", tk)
}

// playAnim aplica os quadros da animação em curso até ela acabar, checando a
// view a cada quadro; devolve quantos quadros houve.
func playAnim(t *testing.T, m Model, check func(view string)) (Model, int) {
	t.Helper()
	frames := 0
	for m.anim.kind != animNone {
		if frames++; frames > 100 {
			t.Fatal("animação não termina")
		}
		check(ansi.Strip(m.View()))
		m, _ = m.Update(animMsg{seq: m.anim.seq})
	}
	return m, frames
}

func TestAnimationsDecorateWithoutHidingText(t *testing.T) {
	m := loaded(t, nil)

	m = pick(t, m, errado...)
	if m.anim.kind != animShake {
		t.Fatalf("erro deveria tremer: %+v", m.anim)
	}
	m, n := playAnim(t, m, func(v string) {
		if !strings.Contains(v, "MOLINETE") || !strings.Contains(v, missNotice) {
			t.Fatalf("tremida escondeu texto:\n%s", v)
		}
	})
	if n != len(shakeOffsets) {
		t.Fatalf("tremida com %d quadros", n)
	}

	m = pick(t, m, pesca...)
	if m.anim.kind != animReveal {
		t.Fatalf("acerto deveria passar o brilho: %+v", m.anim)
	}
	m, _ = playAnim(t, m, func(v string) {
		if !strings.Contains(v, "EQUIPAMENTOS DE PESCA") || !strings.Contains(v, "molinete, vara, linha, anzol") {
			t.Fatalf("brilho escondeu a faixa:\n%s", v)
		}
	})

	m = pick(t, m, armadura...)
	m = pick(t, m, moldavel...)
	m = pick(t, m, truco...)
	if m.phase != phaseFinished || m.anim.kind != animWin {
		t.Fatalf("vitória: phase %d anim %+v", m.phase, m.anim)
	}
	// Voltar do menu no meio da animação a retoma.
	if m.Resume() == nil {
		t.Fatal("Resume deveria religar a animação")
	}
	_, n = playAnim(t, m, func(v string) {
		if !strings.Contains(v, "Parabéns!") || !strings.Contains(v, "TRUCO") {
			t.Fatalf("cascata escondeu o resultado:\n%s", v)
		}
	})
	if n != winFrames {
		t.Fatalf("cascata com %d quadros, quer %d", n, winFrames)
	}
}

func TestShakeMovesOnlyMissedTiles(t *testing.T) {
	m := loaded(t, nil)
	m = pick(t, m, errado...)
	m.anim.frame = 1 // shakeOffsets[1] = 2
	if lk := m.tileLook(slices.Index(m.session.Items(), "vara"), "vara"); lk.offset != 2 || lk.front != missFront {
		t.Fatalf("peça errada: %+v", lk)
	}
	if lk := m.tileLook(slices.Index(m.session.Items(), "zap"), "zap"); lk.offset != 1 || lk.front != cream {
		t.Fatalf("peça fora do chute não deveria tremer: %+v", lk)
	}
}
