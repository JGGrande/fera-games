package feramusicasui

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"fera-games/internal/audio"
	"fera-games/internal/game"
	"fera-games/internal/games/feramusicas"
	"fera-games/internal/games/feramusicas/feramusicastest"
)

var fixedNow = func() time.Time { return time.Date(2026, 11, 15, 12, 0, 0, 0, time.Local) }

// window é quanto settle espera a caixa de entrada ficar quieta. Download e
// decode do áudio falso levam milissegundos; blink do cursor (530 ms) e tick
// do player (100 ms) são ignorados. O que chegar depois não se perde: fica na
// caixa e é processado na próxima rodada (ou por waitFor), como no app real.
const window = 150 * time.Millisecond

// harness conduz o Model como o Bubble Tea faria, rodando os comandos.
type harness struct {
	t       *testing.T
	m       Model
	back    bool
	quit    bool
	src     *feramusicastest.Source
	cat     feramusicas.Catalog
	inbox   chan tea.Msg // resultado de todo comando lançado, mesmo atrasado
	pending atomic.Int64 // comandos lançados que ainda não entregaram
}

func newHarness(t *testing.T, cat feramusicas.Catalog) *harness {
	t.Helper()
	src := &feramusicastest.Source{Seconds: 2}
	h := &harness{t: t, src: src, cat: cat, inbox: make(chan tea.Msg, 4096)}
	h.m = New(Deps{
		Game: feramusicas.NewGame(cat, src), Player: audio.NewSilent(),
		Now: fixedNow, Timeout: 5 * time.Second,
		CatalogNotice: memoryNotice, // como em produção sem Firebase
	})
	h.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	return h
}

// seeded abre o Fera Músicas com o catálogo de exemplo (Pop 1, Rock 2) já
// carregado e o áudio da 1ª categoria no player.
func seeded(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, feramusicastest.NewCatalog(t))
	h.start()
	h.waitAudio()
	return h
}

// start executa o Init.
func (h *harness) start() { h.settle(h.m.Init()) }

// send entrega mensagens ao Model e roda os comandos que elas geram.
func (h *harness) send(msgs ...tea.Msg) {
	for _, msg := range msgs {
		var cmd tea.Cmd
		h.m, cmd = h.m.Update(msg)
		h.settle(cmd)
	}
}

// launch roda cada cmd numa goroutine; o resultado vai para a caixa de entrada.
func (h *harness) launch(cmds ...tea.Cmd) {
	for _, c := range cmds {
		if c != nil {
			h.pending.Add(1)
			go func() {
				h.inbox <- c()
				h.pending.Add(-1) // só depois de entregar: pending 0 ⇒ tudo está na caixa
			}()
		}
	}
}

// settle roda cmds e processa o que chegar até a caixa ficar quieta por window.
func (h *harness) settle(cmds ...tea.Cmd) {
	h.launch(cmds...)
	for {
		if h.pending.Load() == 0 && len(h.inbox) == 0 {
			return // nada em andamento: não há o que esperar
		}
		select {
		case msg := <-h.inbox:
			h.handle(msg)
		case <-time.After(window):
			return
		}
	}
}

// waitFor processa a caixa de entrada até cond valer (ou falha em 5 s).
func (h *harness) waitFor(what string, cond func() bool) {
	h.t.Helper()
	timeout := time.After(5 * time.Second)
	for !cond() {
		select {
		case msg := <-h.inbox:
			h.handle(msg)
		case <-timeout:
			h.t.Fatalf("esperando %s:\n%s", what, h.view())
		}
	}
}

// waitAudio espera o áudio da categoria aberta chegar ao player.
func (h *harness) waitAudio() {
	h.t.Helper()
	h.waitFor("o áudio carregar", func() bool { return h.m.session != nil && h.m.inPlayer == h.m.session })
}

// waitView espera a tela conter o texto.
func (h *harness) waitView(want string) {
	h.t.Helper()
	h.waitFor(fmt.Sprintf("%q na tela", want), func() bool { return strings.Contains(h.view(), want) })
}

// handle aplica uma mensagem vinda de um comando.
func (h *harness) handle(msg tea.Msg) {
	switch msg := msg.(type) {
	case nil, tickMsg, spinner.TickMsg:
		return
	case BackMsg:
		h.back = true
		return
	case tea.QuitMsg:
		h.quit = true
		return
	case tea.BatchMsg:
		h.launch(msg...)
		return
	}
	if strings.HasSuffix(reflect.TypeOf(msg).PkgPath(), "/cursor") {
		return
	}
	var cmd tea.Cmd
	h.m, cmd = h.m.Update(msg)
	h.launch(cmd)
}

// typeText digita texto como teclas.
func (h *harness) typeText(s string) {
	for _, r := range s {
		h.sendFast(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// sendFast é send para teclas que só geram timers (blink do cursor): não vale
// esperar a janela inteira por elas.
func (h *harness) sendFast(msg tea.Msg) {
	var cmd tea.Cmd
	h.m, cmd = h.m.Update(msg)
	if cmd != nil {
		go cmd() // descartado: roda e é ignorado, sem bloquear
	}
}

func (h *harness) press(msgs ...tea.KeyPressMsg) {
	for _, k := range msgs {
		h.send(k)
	}
}

func (h *harness) view() string { return ansi.Strip(h.m.View()) }

func (h *harness) mustContain(want ...string) {
	h.t.Helper()
	v := h.view()
	for _, w := range want {
		if !strings.Contains(v, w) {
			h.t.Fatalf("a tela deveria conter %q:\n%s", w, v)
		}
	}
}

func (h *harness) mustNotContain(bad ...string) {
	h.t.Helper()
	v := h.view()
	for _, w := range bad {
		if strings.Contains(v, w) {
			h.t.Fatalf("a tela não deveria conter %q:\n%s", w, v)
		}
	}
}

var (
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	tab   = tea.KeyPressMsg{Code: tea.KeyTab}
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
	space = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
)

func ch(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// memoryNotice espelha o aviso que o app passa quando não há Firebase.
const memoryNotice = "sem Firebase configurado (.env): o catálogo fica só em memória e some ao fechar"

// played diz se o player recebeu a ordem de tocar: ainda está tocando ou já
// chegou ao fim do trecho. O round 1 tem só 0,13 s e pode acabar antes da
// verificação (ainda mais com -race e testes em paralelo).
func played(p audio.Player) bool {
	return p.Playing() || (p.Duration() > 0 && p.Position() >= p.Duration())
}

const popAnswer = "Michael Jackson - Billie Jean" // única música de Pop

// --- abertura ---------------------------------------------------------------

func TestCatalogoVazioMandaCadastrarNoAdmin(t *testing.T) {
	t.Parallel()
	h := newHarness(t, feramusicas.NewMemoryCatalog())
	h.start()
	if h.m.screen != screenEmpty {
		t.Fatalf("screen = %d, quero a tela de catálogo vazio", h.m.screen)
	}
	h.mustContain("Nenhuma música cadastrada", "Admin", "tecla a no menu inicial", "só em memória")
	if len(h.src.Fetched()) != 0 {
		t.Fatal("sem músicas não há nada para baixar")
	}
	h.press(ch('a')) // a tecla do Admin é do menu inicial, não daqui
	if h.m.screen != screenEmpty || h.back {
		t.Fatal("a não faz nada dentro do jogo")
	}

	// Com Firestore não há aviso de memória.
	f := New(Deps{Game: h.m.deps.Game, Player: audio.NewSilent(), Now: fixedNow})
	f, _ = f.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	f.screen = screenEmpty
	if v := ansi.Strip(f.View()); strings.Contains(v, "memória") {
		t.Fatalf("sem CatalogNotice a tela não deveria falar em memória:\n%s", v)
	}

	// Cadastrada uma música (pelo Admin), voltar ao jogo já abre a partida.
	_, _ = h.cat.Add(context.Background(), feramusicas.Song{Name: "A", Category: "Rock", URL: "https://youtu.be/aaaaaaaaaaa"})
	h.press(ch('q'))
	if !h.back {
		t.Fatal("q deveria voltar ao menu")
	}
	h.settle(h.m.Resume())
	if h.m.screen != screenGame || h.m.session == nil {
		t.Fatalf("depois de cadastrar, retomar deveria abrir o jogo: screen=%d", h.m.screen)
	}
}

func TestMNaoAbreMaisOCatalogo(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('m'))
	if h.m.screen != screenGame {
		t.Fatalf("m não deveria sair do jogo: screen=%d", h.m.screen)
	}
	// A lista de músicas (que entrega as respostas) não aparece no jogo.
	h.mustNotContain("Nirvana", "Queen", "cadastrar")
	h.press(ch('?'))
	h.mustNotContain("m músicas", "m      músicas")
}

func TestComCatalogoAbreOJogoETocaOTrecho(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	if h.m.screen != screenGame || h.m.session == nil {
		t.Fatalf("screen = %d session = %v", h.m.screen, h.m.session != nil)
	}
	h.mustContain("Fera Músicas", "2026-11-15", "1 Pop", "2 Rock", "sua vez", "0.1s", "0.5s", "15s")
	if got := h.src.Fetched(); len(got) != 1 {
		t.Fatalf("deveria ter baixado só a categoria aberta: %v", got)
	}
	if h.m.inPlayer != h.m.session || !played(h.m.deps.Player) {
		t.Fatal("o trecho deveria estar carregado e tocando")
	}
	h.mustNotContain(popAnswer) // nada de spoiler
}

func TestQVoltaAoMenuEPausa(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('q'))
	if !h.back || h.m.deps.Player.Playing() {
		t.Fatalf("back=%v playing=%v", h.back, h.m.deps.Player.Playing())
	}
}

func TestFalhaAoLerOCatalogoMostraErroERetry(t *testing.T) {
	t.Parallel()
	cat := &flakyCatalog{MemoryCatalog: feramusicastest.NewCatalog(t), fail: true}
	h := newHarness(t, cat)
	h.start()
	h.mustContain("Não deu para abrir o catálogo", "banco fora do ar")
	cat.fail = false
	h.press(ch('r'))
	if h.m.screen != screenGame {
		t.Fatalf("screen = %d depois do retry", h.m.screen)
	}
}

type flakyCatalog struct {
	*feramusicas.MemoryCatalog
	fail bool
}

func (c *flakyCatalog) List(ctx context.Context) ([]feramusicas.Song, error) {
	if c.fail {
		return nil, errors.New("banco fora do ar")
	}
	return c.MemoryCatalog.List(ctx)
}

// --- jogo -------------------------------------------------------------------

func TestPularAvancaORoundEPrendeNoLimite(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(tab)
	if st := h.m.session.State(); st.Step != 2 {
		t.Fatalf("pular não avançou: %+v", st)
	}
	h.mustContain("pulou", "⬜")
	if !played(h.m.deps.Player) {
		t.Fatal("o novo round deveria tocar do início")
	}
	// O player recebeu o trecho já cortado no limite do round (530 ms): ir além
	// prende no fim e para sozinho, sem depender do tick.
	if d := h.m.deps.Player.Duration(); d != feramusicas.LimitFor(2) {
		t.Fatalf("duração no player = %v, quero o limite %v", d, feramusicas.LimitFor(2))
	}
	h.m.deps.Player.SeekTo(time.Second)
	if h.m.deps.Player.Playing() {
		t.Fatal("o player deveria parar no fim do trecho do round")
	}
	if pos := h.m.deps.Player.Position(); pos != feramusicas.LimitFor(2) {
		t.Fatalf("posição = %v, quero %v", pos, feramusicas.LimitFor(2))
	}
	// Espaço depois do fim recomeça o trecho do início.
	h.press(space)
	if !played(h.m.deps.Player) {
		t.Fatal("espaço no fim do trecho deveria tocar de novo do início")
	}
}

func TestCadaRoundEntregaOTrechoCortadoNoLimite(t *testing.T) {
	t.Parallel()
	h := newHarness(t, feramusicastest.NewCatalog(t))
	h.src.Seconds = 2.5 // rounds 1-3 cortam (0.13, 0.53, 2 s); do 4º em diante é a faixa toda
	h.start()
	h.waitAudio()
	for round := 1; round <= 3; round++ {
		if d := h.m.deps.Player.Duration(); d != feramusicas.LimitFor(round) {
			t.Fatalf("round %d: trecho de %v, quero %v", round, d, feramusicas.LimitFor(round))
		}
		h.press(tab)
	}
	if d := h.m.deps.Player.Duration(); d != 2500*time.Millisecond {
		t.Fatalf("round 4: trecho de %v, quero a faixa toda (2,5 s)", d)
	}
	h.press(tab, tab) // round 5 e, no 5º pulo, fim da partida
	if h.m.session.State().Status != game.Lost {
		t.Fatalf("status = %v, quero Lost", h.m.session.State().Status)
	}
	if h.m.deps.Player.Playing() || h.m.deps.Player.Duration() != 2500*time.Millisecond {
		t.Fatalf("no fim, o trecho inteiro fica carregado e pausado (playing=%v dur=%v)",
			h.m.deps.Player.Playing(), h.m.deps.Player.Duration())
	}
}

func TestSeekNaoPassaDoLimite(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(tab, tab) // round 3: limite de 2 s; o áudio falso também tem 2 s
	h.m.deps.Player.Pause()
	h.m.deps.Player.SeekTo(feramusicas.LimitFor(3) - 200*time.Millisecond)
	h.press(tea.KeyPressMsg{Code: tea.KeyRight})
	if pos := h.m.deps.Player.Position(); pos > feramusicas.LimitFor(3) {
		t.Fatalf("seek passou do limite: %v", pos)
	}
}

func TestDigitarEAcertar(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(enter)
	if !h.m.typing {
		t.Fatal("enter deveria abrir a digitação")
	}
	h.typeText("billie")
	if len(h.m.matches) == 0 || h.m.matches[0] != popAnswer {
		t.Fatalf("matches = %v", h.m.matches)
	}
	h.mustContain("▸ "+popAnswer, "enter")
	h.press(enter)

	if st := h.m.session.State(); st.Status != game.Won {
		t.Fatalf("status = %v", st.Status)
	}
	h.mustContain("Acertou!", popAnswer, "🟩")
	if h.m.deps.Player.Playing() {
		t.Fatal("ao acertar o player pausa")
	}
	if h.m.typing {
		t.Fatal("a digitação deveria ter fechado")
	}
}

func TestDigitarSemSugestoesEhFuzzyECompletaComTab(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(enter)
	h.typeText("queen")
	if len(h.m.matches) != 1 || !strings.Contains(h.m.matches[0], "Queen") {
		t.Fatalf("matches = %v", h.m.matches)
	}
	h.press(tab)
	if got := h.m.input.Value(); got != "Queen - Bohemian Rhapsody" {
		t.Fatalf("tab deveria completar, input = %q", got)
	}
	h.press(enter) // chuta Queen (errado em Pop)
	if len(h.m.session.Board()) != 1 || h.m.session.State().Status != game.Playing {
		t.Fatalf("board = %+v", h.m.session.Board())
	}
	h.mustContain("🟥", "Queen - Bohemian Rhapsody")
}

func TestSetasEscolhemASugestao(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(enter)
	h.typeText(" - ") // casa com as 3 músicas
	if len(h.m.matches) != 3 {
		t.Fatalf("matches = %v", h.m.matches)
	}
	h.press(down)
	if h.m.sel != 1 {
		t.Fatalf("sel = %d", h.m.sel)
	}
	want := h.m.matches[1]
	h.press(enter)
	if got := h.m.session.State().Guesses[0]; got != want {
		t.Fatalf("chutou %q, quero %q", got, want)
	}
}

func TestTextoForaDoCatalogoMantemADigitacao(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(enter)
	h.typeText("zzzzzz")
	h.mustContain("nenhuma música parecida")
	h.press(enter)
	if !h.m.typing || len(h.m.session.Board()) != 0 {
		t.Fatalf("typing=%v board=%v", h.m.typing, h.m.session.Board())
	}
	h.mustContain("escolha uma música da lista")
}

func TestEscCancelaADigitacao(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(enter)
	h.typeText("bill")
	h.press(esc)
	if h.m.typing || h.back {
		t.Fatalf("esc só fecha a digitação: typing=%v back=%v", h.m.typing, h.back)
	}
}

func TestPerderEmCincoRevelaARespostaEPermiteCopiar(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	for i := 0; i < 5; i++ {
		h.press(tab)
	}
	if st := h.m.session.State(); st.Status != game.Lost {
		t.Fatalf("status = %v", st.Status)
	}
	h.mustContain("Não foi dessa vez.", popAnswer, "⬜⬜⬜⬜⬜")
	if h.m.deps.Player.Playing() {
		t.Fatal("player deveria estar pausado")
	}
	// Terminada a partida, pular/chutar não fazem nada.
	h.press(tab, enter)
	if len(h.m.session.Board()) != 5 || h.m.typing {
		t.Fatal("partida terminada não aceita mais chutes")
	}

	// c copia o resultado (não executamos o comando: ele escreveria no clipboard real).
	m, cmd := h.m.Update(ch('c'))
	if cmd == nil || !m.copied {
		t.Fatalf("c não pediu a cópia (copied=%v cmd=%v)", m.copied, cmd != nil)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "resultado copiado") {
		t.Fatalf("sem aviso de cópia:\n%s", v)
	}
}

func TestCopiarSoNoFim(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	if _, cmd := h.m.Update(ch('c')); cmd != nil {
		t.Fatal("c durante a partida não deveria copiar")
	}
}

func TestEspacoAlternaPlayPause(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(tab, tab, tab) // round 4: o trecho tem os 2 s do áudio falso, não acaba no meio do teste
	h.press(space)
	if h.m.deps.Player.Playing() {
		t.Fatal("espaço deveria pausar")
	}
	h.press(space)
	if !h.m.deps.Player.Playing() {
		t.Fatal("espaço deveria tocar")
	}
}

// --- categorias -------------------------------------------------------------

func TestTrocarDeCategoriaBaixaUmaVezSo(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('2'))
	h.waitAudio()
	if h.m.cur != "rock" || h.m.session.Category() != "Rock" {
		t.Fatalf("cur = %q", h.m.cur)
	}
	h.mustContain("sua vez")
	if got := h.src.Fetched(); len(got) != 2 || got[0] == got[1] {
		t.Fatalf("Fetched = %v", got)
	}
	if h.m.inPlayer != h.m.session || !played(h.m.deps.Player) {
		t.Fatal("o áudio de Rock deveria estar tocando")
	}

	h.press(tab) // joga na categoria Rock...
	h.press(ch('1'))
	if h.m.cur != "pop" || len(h.m.session.Board()) != 0 {
		t.Fatalf("Pop deveria estar intacta: cur=%q board=%v", h.m.cur, h.m.session.Board())
	}
	h.press(ch('2')) // ...e volta: a partida continua de onde parou, sem baixar de novo.
	if len(h.m.session.Board()) != 1 {
		t.Fatalf("a partida de Rock deveria ter sido mantida: %v", h.m.session.Board())
	}
	if got := h.src.Fetched(); len(got) != 2 {
		t.Fatalf("voltar para uma categoria já baixada não pode baixar de novo: %v", got)
	}
}

func TestCategoriaInexistenteEIgnorada(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('7'), ch('0'))
	if h.m.cur != "pop" {
		t.Fatalf("cur = %q", h.m.cur)
	}
}

func TestTrocarParaCategoriaTerminadaNaoToca(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	for i := 0; i < 5; i++ {
		h.press(tab)
	}
	h.press(ch('2'), ch('1')) // sai e volta para a partida terminada
	if h.m.deps.Player.Playing() {
		t.Fatal("partida terminada deveria carregar o áudio pausado")
	}
	h.mustContain("Não foi dessa vez.")
}

// --- falhas de áudio --------------------------------------------------------

func TestSemFerramentasMostraOQueInstalarESemTeclaR(t *testing.T) {
	t.Parallel()
	cat := feramusicastest.NewCatalog(t)
	h := newHarness(t, cat)
	h.src.Err = fmt.Errorf("%w: yt-dlp, ffmpeg", feramusicas.ErrMissingTools)
	h.start()
	h.waitView("faltando")
	h.mustContain("faltando yt-dlp, ffmpeg", "README", "reabra o programa")
	h.mustNotContain("tecle r")
	if h.m.inPlayer != nil {
		t.Fatal("não deveria haver áudio carregado")
	}
	// A partida segue jogável sem áudio.
	h.press(tab)
	if h.m.session.State().Step != 2 {
		t.Fatal("pular deveria funcionar sem áudio")
	}
	h.press(space) // sem áudio, espaço não pode quebrar

	// r não existe mais no jogo: não dispara outro download.
	h.src.Err = nil
	before := len(h.src.Fetched())
	h.press(ch('r'))
	if len(h.src.Fetched()) != before || h.m.inPlayer != nil {
		t.Fatal("r não deveria fazer nada no jogo")
	}
	// Sair da categoria e voltar tenta de novo.
	h.press(ch('2'), ch('1'))
	h.waitAudio()
	if h.m.inPlayer != h.m.session {
		t.Fatal("voltar à categoria deveria baixar o áudio de novo")
	}
	h.mustNotContain("faltando")
}

func TestAjudaDoJogoNaoMostraR(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('?'))
	h.mustNotContain("tentar de novo")
}

func TestErrosDeDownloadViramFrasesAcionaveis(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want string
	}{
		{feramusicas.ErrVideoUnavailable, "vídeo indisponível; remova essa música"},
		{feramusicas.ErrAgeRestricted, "restrição de idade"},
		{fmt.Errorf("%w; atualize", feramusicas.ErrBotCheck), "yt-dlp -U"},
		{context.DeadlineExceeded, "demorou demais"},
		{fmt.Errorf("%w (10:00): confira", feramusicas.ErrStartPastEnd), "corrija o início"},
		{feramusicas.ErrNoAudio, "o vídeo não tem áudio"},
		{errors.New("ffmpeg falhou: 403 Forbidden\nmais lixo"), "download falhou: ffmpeg falhou: 403 Forbidden"},
	}
	for _, tc := range cases {
		if got := friendlyAudio(tc.err); !strings.Contains(got, tc.want) {
			t.Errorf("friendlyAudio(%v) = %q, quero conter %q", tc.err, got, tc.want)
		}
	}
}

// --- catálogo ---------------------------------------------------------------

func TestRemoverUltimaMusicaDeUmaCategoriaTiraAAba(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	ctx := context.Background()
	all, _ := h.cat.List(ctx)
	for _, s := range all {
		if s.Category == "Pop" {
			_ = h.cat.Remove(ctx, s.ID) // pelo Admin
		}
	}
	h.settle(h.m.Resume()) // de volta ao jogo
	if len(h.m.cats) != 1 || h.m.cats[0] != "Rock" {
		t.Fatalf("cats = %v", h.m.cats)
	}
	h.mustContain("1 Rock")
	h.mustNotContain("Pop")
	if h.m.cur != "rock" || h.m.session == nil || h.m.session.Category() != "Rock" {
		t.Fatalf("o jogo deveria abrir em Rock: cur=%q", h.m.cur)
	}
}

// --- cadastro ---------------------------------------------------------------

func TestCadastroDeNovaCategoriaApareceNasAbas(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	_, _ = h.cat.Add(context.Background(), feramusicas.Song{Name: "Anitta - Envolver", Category: "Funk", URL: "https://youtu.be/aaaaaaaaaaa"})
	h.settle(h.m.Resume())
	h.mustContain("1 Funk", "2 Pop", "3 Rock")
}

// --- tamanho ----------------------------------------------------------------

func TestTelasCabemEm80x24(t *testing.T) {
	t.Parallel()
	check := func(name string, h *harness) {
		t.Helper()
		v := h.m.View()
		lines := strings.Split(v, "\n")
		if len(lines) > 24 {
			t.Errorf("%s: %d linhas (máx. 24):\n%s", name, len(lines), ansi.Strip(v))
		}
		for _, l := range lines {
			if w := lipgloss.Width(l); w > 80 {
				t.Errorf("%s: linha com %d colunas: %q", name, w, ansi.Strip(l))
			}
		}
	}

	h := seeded(t)
	check("jogo", h)

	h.press(enter)
	h.typeText(" - ")
	check("digitando com 3 sugestões", h)
	h.press(esc)

	h.press(ch('?'))
	check("jogo com ajuda completa", h)
	h.press(ch('?'))

	e := newHarness(t, feramusicas.NewMemoryCatalog())
	e.start()
	check("catálogo vazio", e)

	for i := 0; i < 5; i++ {
		h.press(tab)
	}
	check("partida perdida", h)

	f := newHarness(t, feramusicastest.NewCatalog(t))
	f.src.Err = fmt.Errorf("%w: yt-dlp, ffmpeg", feramusicas.ErrMissingTools)
	f.start()
	f.waitView("faltando")
	check("sem ferramentas", f)

	// Muitas categorias com nomes longos: o cabeçalho trunca em vez de quebrar a linha.
	cat := feramusicas.NewMemoryCatalog()
	for i := 0; i < 12; i++ {
		_, _ = cat.Add(context.Background(), feramusicas.Song{
			Name: "x", Category: fmt.Sprintf("Categoria muito comprida número %02d", i),
			URL: fmt.Sprintf("https://youtu.be/%011d", i),
		})
	}
	g := newHarness(t, cat)
	g.start()
	check("muitas categorias", g)
	// Só as 3 categorias do dia viram abas.
	g.mustContain("1 Categoria", "3 Categoria")
	g.mustNotContain("4 Categoria")
}

func TestMenorQue80ColunasNaoQuebraOUpdate(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.send(tea.WindowSizeMsg{Width: 40, Height: 10})
	_ = h.m.View() // só não pode dar pânico
}

// --- downloads por sessão (bug 6) e catálogo vivo (bug 5) -------------------

func TestTrocarDeAbaNoMeioDoDownloadNaoBaixaDeNovo(t *testing.T) {
	t.Parallel()
	h := newHarness(t, feramusicastest.NewCatalog(t))
	gate := make(chan struct{})
	defer close(gate)
	h.src.Gate = gate // downloads ficam presos até o fim do teste
	h.start()
	pop := h.m.session
	h.mustContain("baixando o trecho do YouTube")

	h.press(ch('2')) // Rock começa a baixar...
	h.press(ch('1')) // ...e Pop continua baixando: não pode disparar outro download.
	if got := h.src.Fetched(); len(got) != 2 {
		t.Fatalf("Fetched = %v, quero 1 download de Pop e 1 de Rock", got)
	}
	if h.m.session != pop || !h.m.loading[pop] {
		t.Fatal("Pop deveria seguir marcada como baixando")
	}
	h.mustContain("baixando o trecho do YouTube")
}

func TestErroDeAudioFicaNaSuaCategoria(t *testing.T) {
	t.Parallel()
	h := seeded(t) // Pop baixou
	pop := h.m.session
	h.src.Err = feramusicas.ErrVideoUnavailable
	h.press(ch('2')) // Rock falha
	rock := h.m.session
	h.waitView("vídeo indisponível")
	h.press(ch('1'))
	if h.m.session != pop || h.m.inPlayer != pop {
		t.Fatal("Pop deveria voltar com o áudio dela")
	}
	h.mustNotContain("vídeo indisponível") // o erro é de Rock, não de Pop
	h.press(ch('2'))
	h.mustContain("vídeo indisponível")
	h.src.Err = nil
	h.press(ch('q')) // sair e entrar de novo tenta de novo
	h.settle(h.m.Resume())
	h.waitAudio()
	if h.m.inPlayer != rock {
		t.Fatal("voltar a Rock deveria baixar o áudio de novo")
	}
	h.mustNotContain("vídeo indisponível")
}

func TestDownloadAtrasadoDeCategoriaRemovidaEIgnorado(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	old := h.m.session // Pop
	all, _ := h.cat.List(context.Background())
	for _, s := range all {
		if s.Category == "Pop" {
			_ = h.cat.Remove(context.Background(), s.ID)
		}
	}
	h.settle(h.m.Resume())
	h.send(trackMsg{session: old, err: errors.New("atrasado")})
	if _, ok := h.m.audioErrs[old]; ok {
		t.Fatal("resposta de sessão descartada não pode ser guardada")
	}
}

func TestMusicaCadastradaNoMeioDaPartidaApareceNoAutocomplete(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(tab) // partida de Pop em andamento
	_, _ = h.cat.Add(context.Background(), feramusicas.Song{
		Name: "Daft Punk - One More Time", Category: "Eletrônica", URL: "https://youtu.be/FGBhQbmPwH8",
	})
	h.press(ch('q'))
	h.settle(h.m.Resume()) // volta do Admin para o jogo
	if h.m.session == nil || h.m.session.Category() != "Pop" || len(h.m.session.Board()) != 1 {
		t.Fatal("deveria voltar para a mesma partida de Pop")
	}
	h.press(enter)
	h.typeText("daft")
	if len(h.m.matches) == 0 || h.m.matches[0] != "Daft Punk - One More Time" {
		t.Fatalf("matches = %v, quero a música nova", h.m.matches)
	}
	h.press(enter)
	if got := h.m.session.State().Guesses; len(got) != 2 || got[1] != "Daft Punk - One More Time" {
		t.Fatalf("Guesses = %v", got)
	}
}

// --- 3 músicas por dia ------------------------------------------------------

// fiveCategories é um catálogo com 5 categorias de 1 música cada.
func fiveCategories(t *testing.T) *feramusicas.MemoryCatalog {
	t.Helper()
	cat := feramusicas.NewMemoryCatalog()
	for i, c := range []string{"Axé", "Funk", "Pop", "Rock", "Samba"} {
		_, err := cat.Add(context.Background(), feramusicas.Song{
			Name: "Música de " + c, Category: c, URL: fmt.Sprintf("https://youtu.be/%011d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return cat
}

func TestAbasSaoAsTresCategoriasDoDia(t *testing.T) {
	t.Parallel()
	cat := fiveCategories(t)
	h := newHarness(t, cat)
	h.start()
	all, _ := cat.Categories(context.Background())
	want := feramusicas.DailyCategories(fixedNow(), all)
	if len(want) != 3 || !slices.Equal(h.m.daily, want) {
		t.Fatalf("daily = %v, quero %v", h.m.daily, want)
	}
	for i, c := range want {
		h.mustContain(fmt.Sprintf("%d %s", i+1, c))
	}
	for _, c := range all {
		if !slices.Contains(want, c) {
			h.mustNotContain(" " + c + " ") // fora do dia, não vira aba
		}
	}
	if h.m.session.Category() != want[0] {
		t.Fatalf("abre em %q, quero a 1ª do dia %q", h.m.session.Category(), want[0])
	}
	h.press(ch('3'))
	if h.m.session.Category() != want[2] {
		t.Fatalf("3 abre %q, quero %q", h.m.session.Category(), want[2])
	}
	h.press(ch('4')) // não há 4ª música no dia
	if h.m.session.Category() != want[2] {
		t.Fatal("4 não deveria trocar de categoria")
	}
	h.mustContain("0/3 músicas do dia")
}

func TestProgressoECopiaDoDia(t *testing.T) {
	t.Parallel()
	h := seeded(t) // Pop e Rock: o dia tem 2 músicas
	for range 5 {
		h.press(tab)
	}
	h.mustContain("1/2 músicas do dia")
	// Com uma só concluída, c copia o resultado dela.
	if got := h.m.copyText(); !strings.Contains(got, "[Pop] X/5") {
		t.Fatalf("copyText = %q", got)
	}

	h.press(ch('2'), enter)
	h.typeText("queen")
	h.press(enter)
	if h.m.session.State().Status == game.Playing {
		for range 5 {
			h.press(tab)
		}
	}
	h.mustContain("2/2 músicas do dia")
	got := h.m.copyText()
	if !strings.HasPrefix(got, "Fera Músicas 2026-11-15 ") || !strings.Contains(got, "/2\n") ||
		!strings.Contains(got, "[Pop]") || !strings.Contains(got, "[Rock]") {
		t.Fatalf("copyText com o dia todo = %q", got)
	}
}

func TestCategoriaQueSaiDoDiaPerdeASessao(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cat := feramusicas.NewMemoryCatalog()
	for i, c := range []string{"A", "B", "C"} {
		_, _ = cat.Add(ctx, feramusicas.Song{Name: "m" + c, Category: c, URL: fmt.Sprintf("https://youtu.be/%011d", i)})
	}
	h := newHarness(t, cat)
	h.start()
	h.press(ch('2'), ch('3'), ch('1')) // abre as 3
	if len(h.m.sessions) != 3 {
		t.Fatalf("sessões = %d", len(h.m.sessions))
	}
	// Procura uma categoria nova que entre no sorteio de hoje (troca uma das 3).
	before := slices.Clone(h.m.daily)
	var added string
	for i := 0; added == "" && i < 200; i++ {
		name := fmt.Sprintf("Nova %d", i)
		if slices.Contains(feramusicas.DailyCategories(fixedNow(), append(slices.Clone(before), name)), name) {
			added = name
			_, _ = cat.Add(ctx, feramusicas.Song{Name: "nova", Category: name, URL: "https://youtu.be/zzzzzzzzzzz"})
		}
	}
	if added == "" {
		t.Skip("nenhuma categoria nova entrou no sorteio")
	}
	h.settle(h.m.reload())
	if len(h.m.daily) != 3 || !slices.Contains(h.m.daily, added) {
		t.Fatalf("daily = %v, quero conter %q", h.m.daily, added)
	}
	if len(h.m.sessions) != 2 {
		t.Fatalf("a sessão da categoria que saiu do dia deveria sumir: %d sessões", len(h.m.sessions))
	}
	for k := range h.m.sessions {
		if !h.m.isDaily(k) {
			t.Fatalf("sessão de %q, que não é do dia", k)
		}
	}
}

// --- início do trecho -------------------------------------------------------

func TestInicioVazioComecaDoZeroESemMarcaNaLista(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	if got := h.src.FetchedSongs(); len(got) != 1 || got[0].Start != 0 {
		t.Fatalf("sem início, o download começa do 0: %+v", got)
	}
}

func TestMusicaComInicioBaixaAPartirDele(t *testing.T) {
	t.Parallel()
	cat := feramusicas.NewMemoryCatalog()
	_, _ = cat.Add(context.Background(), feramusicas.Song{
		Name: "Queen - Bohemian Rhapsody", Category: "Rock", URL: "https://youtu.be/fJ9rUzIMcZQ", Start: 90 * time.Second,
	})
	h := newHarness(t, cat)
	h.start()
	h.waitAudio()
	if got := h.src.FetchedSongs(); len(got) != 1 || got[0].Start != 90*time.Second {
		t.Fatalf("o download deveria pedir o trecho a partir de 1:30: %+v", got)
	}
	h.mustNotContain("1:30") // o início não aparece no jogo
}
