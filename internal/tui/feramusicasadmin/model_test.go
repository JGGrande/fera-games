package feramusicasadmin

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"fera-games/internal/audio"
	"fera-games/internal/games/feramusicas"
	"fera-games/internal/games/feramusicas/feramusicastest"
)

var fixedNow = func() time.Time { return time.Date(2026, 11, 15, 12, 0, 0, 0, time.Local) }

const memoryNotice = "sem Firebase configurado (.env): o catálogo fica só em memória e some ao fechar"

// window é quanto settle espera a caixa de entrada ficar quieta.
const window = 150 * time.Millisecond

// harness conduz o Model como o Bubble Tea faria, rodando os comandos.
type harness struct {
	t       *testing.T
	m       Model
	back    bool
	cat     feramusicas.Catalog
	src     *feramusicastest.Source
	inbox   chan tea.Msg
	pending atomic.Int64
}

func newHarness(t *testing.T, cat feramusicas.Catalog) *harness {
	t.Helper()
	h := &harness{t: t, cat: cat, src: &feramusicastest.Source{Seconds: 3}, inbox: make(chan tea.Msg, 4096)}
	h.m = New(Deps{
		Game: feramusicas.NewGame(cat, h.src), Now: fixedNow, CatalogNotice: memoryNotice,
		Player: audio.NewSilent(), Timeout: 5 * time.Second,
	})
	h.send(tea.WindowSizeMsg{Width: 80, Height: 24})
	h.settle(h.m.Init())
	return h
}

func seeded(t *testing.T) *harness {
	t.Helper()
	return newHarness(t, feramusicastest.NewCatalog(t))
}

func (h *harness) send(msgs ...tea.Msg) {
	for _, msg := range msgs {
		var cmd tea.Cmd
		h.m, cmd = h.m.Update(msg)
		h.settle(cmd)
	}
}

func (h *harness) launch(cmds ...tea.Cmd) {
	for _, c := range cmds {
		if c != nil {
			h.pending.Add(1)
			go func() {
				h.inbox <- c()
				h.pending.Add(-1)
			}()
		}
	}
}

func (h *harness) settle(cmds ...tea.Cmd) {
	h.launch(cmds...)
	for {
		if h.pending.Load() == 0 && len(h.inbox) == 0 {
			return
		}
		select {
		case msg := <-h.inbox:
			h.handle(msg)
		case <-time.After(window):
			return
		}
	}
}

func (h *harness) handle(msg tea.Msg) {
	switch msg := msg.(type) {
	case nil, spinner.TickMsg, tickMsg:
		return
	case BackMsg:
		h.back = true
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

// typeText digita sem esperar: teclas de texto só geram o blink do cursor.
func (h *harness) typeText(s string) {
	for _, r := range s {
		var cmd tea.Cmd
		h.m, cmd = h.m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		if cmd != nil {
			go cmd()
		}
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
	enter    = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc      = tea.KeyPressMsg{Code: tea.KeyEscape}
	tab      = tea.KeyPressMsg{Code: tea.KeyTab}
	shiftTab = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	down     = tea.KeyPressMsg{Code: tea.KeyDown}
	up       = tea.KeyPressMsg{Code: tea.KeyUp}
)

func ch(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// fillForm preenche nome, categoria e link. Se o tab da categoria só completou
// o texto, um segundo tab avança.
func fillForm(h *harness, name, category, link string) {
	h.typeText(name)
	h.press(tab)
	h.typeText(category)
	h.press(tab)
	if h.m.focus != fURL {
		h.press(tab)
	}
	h.typeText(link)
}

// --- lista ------------------------------------------------------------------

func TestCatalogoVazio(t *testing.T) {
	t.Parallel()
	h := newHarness(t, feramusicas.NewMemoryCatalog())
	if h.m.screen != screenList {
		t.Fatalf("screen = %d", h.m.screen)
	}
	h.mustContain("Fera Músicas Admin", "músicas cadastradas (0)", "Nenhuma música cadastrada", "Tecle a", "só em memória")
	h.press(ch('q'))
	if !h.back {
		t.Fatal("q deveria voltar ao menu")
	}
}

func TestListaMostraAsMusicasSemDatas(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.mustContain("músicas cadastradas (3)", "Pop", "Rock",
		"Michael Jackson - Billie Jean", "Queen - Bohemian Rhapsody", "fJ9rUzIMcZQ")
	h.mustNotContain("última", "próxima", "estimativa", "hoje", "amanhã")
}

func TestRemoverComConfirmacao(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	// Ordem: Pop(Billie Jean), Rock(Nirvana, Queen). Desce até Nirvana.
	h.press(down)
	h.press(ch('d'))
	h.mustContain("Remover “Nirvana - Smells Like Teen Spirit”?")
	h.press(ch('n'))
	if n := len(h.m.songs); n != 3 || h.m.confirmDel {
		t.Fatalf("n deveria cancelar: songs=%d confirm=%v", n, h.m.confirmDel)
	}
	h.press(ch('d'), ch('y'))
	if n := len(h.m.songs); n != 2 {
		t.Fatalf("songs = %d, quero 2", n)
	}
	h.mustNotContain("Nirvana")
	if l, _ := h.cat.List(context.Background()); len(l) != 2 {
		t.Fatalf("o catálogo deveria ter perdido a música: %d", len(l))
	}
}

func TestRemoverTudoMostraCatalogoVazio(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	for range 3 {
		h.press(ch('d'), ch('y'))
	}
	h.mustContain("Nenhuma música cadastrada")
	h.press(ch('d')) // nada para remover
	if h.m.confirmDel {
		t.Fatal("sem músicas não há o que confirmar")
	}
}

func TestCursorRespeitaOsLimites(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(up, ch('k'))
	if h.m.cursor != 0 {
		t.Fatalf("cursor = %d", h.m.cursor)
	}
	for range 10 {
		h.press(ch('j'))
	}
	if h.m.cursor != 2 {
		t.Fatalf("cursor = %d, quero 2", h.m.cursor)
	}
}

func TestCatalogoGrandeRolaMantendoOCursorVisivel(t *testing.T) {
	t.Parallel()
	cat := feramusicas.NewMemoryCatalog()
	for i := 0; i < 40; i++ {
		_, err := cat.Add(context.Background(), feramusicas.Song{
			Name: fmt.Sprintf("Música %02d", i), Category: fmt.Sprintf("Cat %d", i%5),
			URL: fmt.Sprintf("https://youtu.be/%011d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	h := newHarness(t, cat)
	for range 25 {
		h.press(ch('j'))
	}
	want := h.m.songs[h.m.cursor].Name
	h.mustContain("▸ "+want, "26/40")
	if n := strings.Count(h.m.View(), "\n") + 1; n > 24 {
		t.Fatalf("a tela tem %d linhas:\n%s", n, h.view())
	}
}

func TestResumeRecarregaOCatalogo(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	// Outra pessoa (ou o próprio jogo, num Firestore compartilhado) mexeu no catálogo.
	_, _ = h.cat.Add(context.Background(), feramusicas.Song{Name: "Nova", Category: "Funk", URL: "https://youtu.be/aaaaaaaaaaa"})
	h.settle(h.m.Resume())
	h.mustContain("músicas cadastradas (4)", "Nova", "Funk")
}

func TestFalhaAoLerOCatalogoMostraErroERetry(t *testing.T) {
	t.Parallel()
	cat := &flakyCatalog{MemoryCatalog: feramusicastest.NewCatalog(t), fail: true}
	h := newHarness(t, cat)
	h.mustContain("Não deu para abrir o catálogo", "banco fora do ar")
	cat.fail = false
	h.press(ch('r'))
	if h.m.screen != screenList || len(h.m.songs) != 3 {
		t.Fatalf("screen = %d songs = %d depois do retry", h.m.screen, len(h.m.songs))
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

// --- cadastro ---------------------------------------------------------------

func TestCadastrarDoZero(t *testing.T) {
	t.Parallel()
	h := newHarness(t, feramusicas.NewMemoryCatalog())
	h.press(ch('a'))
	if h.m.screen != screenForm {
		t.Fatalf("screen = %d", h.m.screen)
	}
	h.mustContain("cadastrar música", "Nome", "Categoria", "Link", "Início")
	h.typeText("Queen - Bohemian Rhapsody")
	h.press(tab)
	h.typeText("Rock")
	h.press(tab)
	h.typeText("https://youtu.be/fJ9rUzIMcZQ")
	h.press(enter)
	h.mustContain("Cadastrar “Queen - Bohemian Rhapsody” em Rock? (y/n)")
	if len(h.m.songs) != 0 {
		t.Fatal("antes de confirmar nada é gravado")
	}
	h.press(ch('y'))

	h.mustContain("✓ cadastrada: Queen - Bohemian Rhapsody")
	if len(h.m.songs) != 1 || h.m.songs[0].VideoID != "fJ9rUzIMcZQ" || h.m.songs[0].Start != 0 {
		t.Fatalf("songs = %+v", h.m.songs)
	}
	// O formulário limpa tudo menos a categoria, para cadastrar em sequência.
	if h.m.fields[fName].Value() != "" || h.m.fields[fURL].Value() != "" || h.m.fields[fCategory].Value() != "Rock" {
		t.Fatalf("campos após salvar: %q %q %q", h.m.fields[fName].Value(), h.m.fields[fCategory].Value(), h.m.fields[fURL].Value())
	}
	if h.m.focus != fName {
		t.Fatalf("foco = %d, quero o nome", h.m.focus)
	}
	h.press(esc) // volta para a lista, já com a música e os dias dela
	h.mustContain("músicas cadastradas (1)", "Queen - Bohemian Rhapsody")
}

func TestCadastroMostraErroNoCampoCerto(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('a'))

	h.press(enter) // tudo vazio
	h.mustContain("informe o nome da música")
	if h.m.focus != fName {
		t.Fatalf("foco = %d", h.m.focus)
	}
	h.typeText("Alguma música")
	h.press(enter)
	h.mustContain("informe a categoria")
	if h.m.focus != fCategory {
		t.Fatalf("foco = %d, quero a categoria", h.m.focus)
	}
	h.typeText("Funk")
	h.press(enter)
	h.mustContain("informe o link do YouTube")
	if h.m.focus != fURL {
		t.Fatalf("foco = %d, quero o link", h.m.focus)
	}
	h.typeText("https://example.com/video")
	h.press(enter)
	h.mustContain("link do YouTube inválido")
	if len(h.m.songs) != 3 {
		t.Fatal("nada deveria ter sido cadastrado")
	}
	h.typeText("x") // digitar de novo limpa o erro
	h.mustNotContain("link do YouTube inválido")
}

func TestCadastroDuplicadoMostraErroNoLink(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('a'))
	fillForm(h, "Outra versão", "Rock", "https://www.youtube.com/watch?v=fJ9rUzIMcZQ&list=PL1") // Bohemian já cadastrada
	h.press(enter, ch('y'))
	h.mustContain("essa música já está cadastrada")
	if h.m.focus != fURL || len(h.m.songs) != 3 {
		t.Fatalf("foco=%d songs=%d", h.m.focus, len(h.m.songs))
	}
}

func TestTabCompletaCategoriaExistente(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('a'), tab) // foco na categoria
	if h.m.focus != fCategory {
		t.Fatalf("foco = %d", h.m.focus)
	}
	h.mustContain("categorias: Pop · Rock")
	h.typeText("ro")
	h.mustContain("categorias: Rock")
	h.mustNotContain("Pop ·")
	h.press(tab)
	if h.m.fields[fCategory].Value() != "Rock" || h.m.focus != fCategory {
		t.Fatalf("tab deveria completar e ficar: valor=%q foco=%d", h.m.fields[fCategory].Value(), h.m.focus)
	}
	h.press(tab) // já é uma categoria existente: agora avança
	if h.m.focus != fURL {
		t.Fatalf("foco = %d, quero o link", h.m.focus)
	}
	h.press(shiftTab)
	if h.m.focus != fCategory {
		t.Fatalf("shift+tab: foco = %d", h.m.focus)
	}
}

func TestTabEmCategoriaNovaAvanca(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('a'), tab)
	h.typeText("Funk")
	h.press(tab)
	if h.m.fields[fCategory].Value() != "Funk" || h.m.focus != fURL {
		t.Fatalf("valor=%q foco=%d", h.m.fields[fCategory].Value(), h.m.focus)
	}
}

func TestEscFechaOCadastroSemSalvar(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('a'))
	h.typeText("rascunho")
	h.press(esc)
	if h.m.screen != screenList || len(h.m.songs) != 3 || h.back {
		t.Fatalf("screen=%d songs=%d back=%v", h.m.screen, len(h.m.songs), h.back)
	}
}

func TestCadastrarComInicio(t *testing.T) {
	t.Parallel()
	h := newHarness(t, feramusicas.NewMemoryCatalog())
	h.press(ch('a'))
	fillForm(h, "Queen - Bohemian Rhapsody", "Rock", "https://youtu.be/fJ9rUzIMcZQ")
	h.press(tab) // campo Início
	if h.m.focus != fStart {
		t.Fatalf("foco = %d, quero o início", h.m.focus)
	}
	h.mustContain("para pular a introdução")
	h.typeText("1:30")
	h.press(enter)
	h.mustContain("em Rock, a partir de 1:30? (y/n)")
	h.press(ch('y'))
	h.mustContain("✓ cadastrada: Queen - Bohemian Rhapsody")
	if len(h.m.songs) != 1 || h.m.songs[0].Start != 90*time.Second {
		t.Fatalf("songs = %+v; quero início de 90 s", h.m.songs)
	}
	if h.m.fields[fStart].Value() != "" {
		t.Fatal("o início deveria ser limpo depois de salvar")
	}
	h.press(esc)
	h.mustContain("fJ9rUzIMcZQ 1:30")
}

func TestInicioInvalidoNaoSalva(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('a'))
	fillForm(h, "Nova", "Rock", "https://youtu.be/aaaaaaaaaaa")
	h.press(tab)
	h.typeText("1:75")
	h.press(enter)
	h.mustContain("início inválido")
	if h.m.focus != fStart || len(h.m.songs) != 3 || h.m.saving {
		t.Fatalf("foco=%d songs=%d saving=%v", h.m.focus, len(h.m.songs), h.m.saving)
	}
	h.press(tea.KeyPressMsg{Code: tea.KeyBackspace}, tea.KeyPressMsg{Code: tea.KeyBackspace})
	h.typeText("15")
	h.press(enter, ch('y'))
	h.mustContain("✓ cadastrada: Nova")
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
	check("lista", h)
	h.press(ch('d'))
	check("lista com confirmação", h)
	h.press(ch('n'), ch('?'))
	check("lista com ajuda completa", h)
	h.press(ch('?'), ch('a'))
	check("cadastro", h)
	h.press(enter)
	check("cadastro com erro", h)
	h.press(tab, tab, tab)
	check("cadastro no início", h)

	// Nomes e categorias longos, início com hora, 30 músicas.
	cat := feramusicas.NewMemoryCatalog()
	for i := 0; i < 30; i++ {
		_, _ = cat.Add(context.Background(), feramusicas.Song{
			Name:     strings.Repeat("Nome muito comprido ", 6),
			Category: fmt.Sprintf("Categoria muito comprida número %02d", i%4),
			URL:      fmt.Sprintf("https://youtu.be/%011d", i), Start: 11*time.Hour + 59*time.Minute + 59500*time.Millisecond,
		})
	}
	check("lista cheia", newHarness(t, cat))
}

// --- prévia do trecho -------------------------------------------------------

var ctrlP = tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}

// waitFor processa a caixa de entrada até cond valer (ou falha em 5 s).
func (h *harness) waitFor(what string, cond func() bool) {
	h.t.Helper()
	timeout := time.After(5 * time.Second)
	for !cond() {
		select {
		case msg := <-h.inbox:
			h.handle(msg)
		case <-time.After(10 * time.Millisecond):
			// Reconfere: cond pode depender de pending, que a goroutine só
			// decrementa depois de entregar a mensagem.
		case <-timeout:
			h.t.Fatalf("esperando %s:\n%s", what, h.view())
		}
	}
}

func (h *harness) waitPreview() {
	h.t.Helper()
	h.waitFor("a prévia baixar", func() bool { return !h.m.previewing })
}

func TestPreviaTocaOTrechoAPartirDoInicio(t *testing.T) {
	t.Parallel()
	h := newHarness(t, feramusicas.NewMemoryCatalog())
	h.press(ch('a'))
	h.mustContain("ctrl+p ouve o trecho", "ctrl+p ouvir")
	fillForm(h, "Queen - Bohemian Rhapsody", "Rock", "https://youtu.be/fJ9rUzIMcZQ")
	h.press(tab)
	h.typeText("1:30")
	h.press(ctrlP)
	h.waitPreview()

	got := h.src.FetchedSongs()
	if len(got) != 1 || got[0].VideoID != "fJ9rUzIMcZQ" || got[0].Start != 90*time.Second {
		t.Fatalf("a prévia deveria baixar o vídeo a partir de 1:30: %+v", got)
	}
	p := h.m.deps.Player
	if !p.Playing() || p.Duration() != 3*time.Second {
		t.Fatalf("a prévia deveria tocar o trecho inteiro: playing=%v dur=%v", p.Playing(), p.Duration())
	}
	h.mustContain("a partir de 1:30 do vídeo", "revelado no round")
	if len(h.m.songs) != 0 {
		t.Fatal("a prévia não salva a música")
	}

	h.press(ctrlP) // para
	if p.Playing() {
		t.Fatal("ctrl+p de novo deveria parar")
	}
	h.press(ctrlP) // toca de novo, sem baixar outra vez
	if !p.Playing() || len(h.src.FetchedSongs()) != 1 {
		t.Fatalf("tocar de novo o mesmo trecho não baixa: playing=%v fetched=%d", p.Playing(), len(h.src.FetchedSongs()))
	}
}

func TestPreviaComOutroInicioBaixaDeNovoEAvisaQueMudou(t *testing.T) {
	t.Parallel()
	h := newHarness(t, feramusicas.NewMemoryCatalog())
	h.press(ch('a'))
	fillForm(h, "x", "Rock", "https://youtu.be/fJ9rUzIMcZQ")
	h.press(ctrlP)
	h.waitPreview()
	h.mustContain("a partir de 0:00 do vídeo")
	h.press(tab) // início
	h.typeText("45")
	h.mustContain("o link ou o início mudou")
	h.m.deps.Player.Pause()
	h.press(ctrlP)
	h.waitPreview()
	got := h.src.FetchedSongs()
	if len(got) != 2 || got[1].Start != 45*time.Second {
		t.Fatalf("outro início é outro trecho: %+v", got)
	}
	h.mustContain("a partir de 0:45 do vídeo")
	h.mustNotContain("mudou")
}

func TestPreviaSemLinkValidoMostraOErroNoCampo(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('a'))
	h.press(ctrlP) // link vazio
	h.mustContain("informe o link do YouTube")
	if h.m.focus != fURL || len(h.src.FetchedSongs()) != 0 {
		t.Fatalf("foco=%d fetched=%d", h.m.focus, len(h.src.FetchedSongs()))
	}
	h.typeText("https://youtu.be/aaaaaaaaaaa")
	h.press(tab)
	h.typeText("1:75")
	h.press(ctrlP)
	h.mustContain("início inválido")
	if h.m.focus != fStart || len(h.src.FetchedSongs()) != 0 {
		t.Fatalf("foco=%d fetched=%d", h.m.focus, len(h.src.FetchedSongs()))
	}
}

func TestPreviaComFalhaExplicaOMotivo(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want string
	}{
		{fmt.Errorf("%w (10:00): confira", feramusicas.ErrStartPastEnd), "o vídeo é mais curto"},
		{feramusicas.ErrVideoUnavailable, "confira o link"},
		{fmt.Errorf("%w: yt-dlp", feramusicas.ErrMissingTools), "faltam o yt-dlp"},
		{context.DeadlineExceeded, "demorou demais"},
		{errors.New("ffmpeg falhou: 403\nlixo"), "download falhou: ffmpeg falhou: 403"},
	}
	for _, tc := range cases {
		h := newHarness(t, feramusicas.NewMemoryCatalog())
		h.src.Err = tc.err
		h.press(ch('a'))
		fillForm(h, "x", "Rock", "https://youtu.be/fJ9rUzIMcZQ")
		h.press(ctrlP)
		h.waitPreview()
		h.mustContain("prévia: ", tc.want)
		if h.m.deps.Player.Playing() {
			t.Fatal("sem trecho não toca nada")
		}
	}
}

func TestSairDoCadastroOuSalvarParaAPrevia(t *testing.T) {
	t.Parallel()
	h := newHarness(t, feramusicas.NewMemoryCatalog())
	h.press(ch('a'))
	fillForm(h, "Queen", "Rock", "https://youtu.be/fJ9rUzIMcZQ")
	h.press(ctrlP)
	h.waitPreview()
	h.press(enter) // salvar: pergunta, e a prévia para
	if h.m.deps.Player.Playing() {
		t.Fatal("salvar deveria parar a prévia")
	}
	h.press(ch('y'))
	h.mustContain("✓ cadastrada: Queen")

	fillForm(h, "Nirvana", "", "https://youtu.be/hTWKbfoikeg") // a categoria Rock ficou do cadastro anterior
	if got := h.m.fields[fCategory].Value(); got != "Rock" {
		t.Fatalf("categoria = %q, quero a mantida (Rock)", got)
	}
	h.press(ctrlP)
	h.waitPreview()
	h.press(esc) // volta para a lista
	if h.m.deps.Player.Playing() {
		t.Fatal("esc deveria parar a prévia")
	}
}

func TestPreviaCanceladaNaoTocaDepois(t *testing.T) {
	t.Parallel()
	h := newHarness(t, feramusicas.NewMemoryCatalog())
	gate := make(chan struct{})
	h.src.Gate = gate
	h.press(ch('a'))
	fillForm(h, "x", "Rock", "https://youtu.be/fJ9rUzIMcZQ")
	h.press(ctrlP)
	h.mustContain("baixando o trecho")
	h.press(esc) // desiste no meio do download
	close(gate)  // o download termina depois
	h.waitFor("o download cancelado terminar", func() bool { return h.pending.Load() == 0 && len(h.inbox) == 0 })
	h.settle()
	if h.m.deps.Player.Playing() || h.m.previewTrack != nil {
		t.Fatal("uma prévia cancelada não pode começar a tocar depois")
	}
}

func TestPreviaCabeEm80x24(t *testing.T) {
	t.Parallel()
	h := newHarness(t, feramusicas.NewMemoryCatalog())
	h.m.deps.AudioNotice = "sem áudio: dispositivo ocupado"
	h.press(ch('a'))
	fillForm(h, "x", "Rock", "https://youtu.be/fJ9rUzIMcZQ")
	h.press(tab)
	h.typeText("11:59:59.5")
	h.press(ctrlP)
	h.waitPreview()
	h.typeText("1") // mudou depois da prévia: aparece o aviso, e o erro do início
	h.press(enter)
	v := h.m.View()
	if n := strings.Count(v, "\n") + 1; n > 24 {
		t.Fatalf("%d linhas:\n%s", n, ansi.Strip(v))
	}
	for _, l := range strings.Split(v, "\n") {
		if w := lipgloss.Width(l); w > 80 {
			t.Fatalf("linha com %d colunas: %q", w, ansi.Strip(l))
		}
	}
}

// --- confirmação, edição e limpar -------------------------------------------

func TestConfirmacaoNaoVoltaAEditarSemPerderNada(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('a'))
	fillForm(h, "Nova", "Funk", "https://youtu.be/aaaaaaaaaaa")
	h.press(enter)
	if !h.m.confirmSave {
		t.Fatal("enter com tudo certo deveria perguntar antes de salvar")
	}
	h.mustContain("Cadastrar “Nova” em Funk? (y/n)", "y confirmar", "n cancelar")
	h.typeText("zzz") // enquanto pergunta, não digita nos campos
	if h.m.fields[fURL].Value() != "https://youtu.be/aaaaaaaaaaa" {
		t.Fatalf("o campo mudou durante a pergunta: %q", h.m.fields[fURL].Value())
	}
	h.press(ch('n'))
	if h.m.confirmSave || h.m.screen != screenForm || len(h.m.songs) != 3 {
		t.Fatalf("n deveria voltar ao formulário sem salvar: confirm=%v screen=%d songs=%d",
			h.m.confirmSave, h.m.screen, len(h.m.songs))
	}
	if h.m.fields[fName].Value() != "Nova" || h.m.fields[fCategory].Value() != "Funk" {
		t.Fatal("voltar da pergunta não pode apagar os campos")
	}
	h.press(enter, esc) // esc também cancela a pergunta (e não fecha o formulário)
	if h.m.screen != screenForm || h.m.confirmSave {
		t.Fatalf("esc na pergunta só cancela: screen=%d", h.m.screen)
	}
	h.press(enter, enter) // enter também confirma
	h.mustContain("✓ cadastrada: Nova")
}

func TestErroDeValidacaoNaoPergunta(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('a'))
	fillForm(h, "Nova", "Funk", "https://example.com")
	h.press(enter)
	if h.m.confirmSave {
		t.Fatal("com link inválido não deveria perguntar")
	}
	h.mustContain("link do YouTube inválido")
	h.mustNotContain("(y/n)")
}

func TestEditarMusica(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(down) // Nirvana (Rock)
	target := h.m.songs[h.m.cursor]
	h.press(ch('e'))
	if h.m.screen != screenForm || h.m.editing == nil || h.m.editing.ID != target.ID {
		t.Fatalf("e deveria abrir a edição de %q", target.Name)
	}
	h.mustContain("editar música", target.Name)
	if h.m.fields[fCategory].Value() != "Rock" || h.m.fields[fURL].Value() != target.URL || h.m.fields[fStart].Value() != "" {
		t.Fatalf("campos = %q %q %q", h.m.fields[fCategory].Value(), h.m.fields[fURL].Value(), h.m.fields[fStart].Value())
	}

	// Muda o nome e o início.
	h.typeText(" (ao vivo)")
	h.press(tab, tab, tab)
	h.typeText("0:42")
	h.press(enter)
	q := strings.Join(strings.Fields(h.view()), " ") // a pergunta pode quebrar em linhas
	if !strings.Contains(q, "Salvar as alterações: “Nirvana - Smells Like Teen Spirit (ao vivo)” em Rock, a partir de 0:42? (y/n)") {
		t.Fatalf("pergunta da edição:\n%s", h.view())
	}
	h.press(ch('y'))

	if h.m.screen != screenList || h.m.editing != nil {
		t.Fatalf("salvar a edição volta à lista: screen=%d", h.m.screen)
	}
	h.mustContain("✓ alterada: Nirvana - Smells Like Teen Spirit (ao vivo)", "hTWKbfoikeg 0:42")
	if len(h.m.songs) != 3 {
		t.Fatalf("editar não cria música nova: %d", len(h.m.songs))
	}
	if got := h.m.songs[h.m.cursor]; got.ID != target.ID || got.Start != 42*time.Second {
		t.Fatalf("o cursor deveria ficar na música editada: %+v", got)
	}
	all, _ := h.cat.List(context.Background())
	for _, s := range all {
		if s.ID == target.ID && s.Name != "Nirvana - Smells Like Teen Spirit (ao vivo)" {
			t.Fatalf("o catálogo não foi alterado: %+v", s)
		}
	}
}

func TestEditarTrocandoParaLinkDeOutraMusica(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('e')) // Billie Jean
	h.press(tab, tab)
	h.press(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}) // apaga o link
	h.typeText("https://youtu.be/fJ9rUzIMcZQ")            // é o da Bohemian
	h.press(enter, ch('y'))
	h.mustContain("essa música já está cadastrada")
	if h.m.screen != screenForm || h.m.focus != fURL || h.m.editing == nil {
		t.Fatalf("o erro deve ficar no formulário, no link: screen=%d foco=%d", h.m.screen, h.m.focus)
	}
}

func TestEditarMusicaRemovidaNoMeioTempo(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('e'))
	_ = h.cat.Remove(context.Background(), h.m.editing.ID) // outra pessoa removeu
	h.press(enter, ch('y'))
	h.mustContain("essa música não existe mais")
}

func TestEscNaEdicaoDescartaENovoCadastroComecaVazio(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('e'))
	h.typeText(" mexido")
	h.press(esc)
	if h.m.screen != screenList || h.m.editing != nil {
		t.Fatal("esc deveria descartar a edição")
	}
	if all, _ := h.cat.List(context.Background()); strings.Contains(all[2].Name, "mexido") || strings.Contains(all[0].Name, "mexido") {
		t.Fatal("esc não pode salvar")
	}
	h.press(ch('a'))
	for i := range h.m.fields {
		if v := h.m.fields[i].Value(); v != "" {
			t.Fatalf("cadastro novo depois de uma edição deveria começar vazio; campo %d = %q", i, v)
		}
	}
	h.mustContain("cadastrar música")
}

func TestRascunhoDoCadastroFicaAoSairEVoltar(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('a'))
	h.typeText("rascunho")
	h.press(esc, ch('a'))
	if h.m.fields[fName].Value() != "rascunho" {
		t.Fatalf("o rascunho de um cadastro novo deveria ficar: %q", h.m.fields[fName].Value())
	}
}

func TestEnterNaListaEdita(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(enter)
	if h.m.editing == nil || h.m.editing.ID != h.m.songs[0].ID {
		t.Fatal("enter na lista deveria editar a música do cursor")
	}
}

func TestCtrlLLimpaOFormulario(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('a'))
	fillForm(h, "Nova", "Funk", "https://example.com")
	h.press(tab)
	h.typeText("1:30")
	h.press(enter) // erro no link
	h.mustContain("link do YouTube inválido")
	h.press(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	for i := range h.m.fields {
		if v := h.m.fields[i].Value(); v != "" {
			t.Fatalf("ctrl+l deveria limpar todos os campos; campo %d = %q", i, v)
		}
	}
	if h.m.formErr != nil || h.m.focus != fName {
		t.Fatalf("ctrl+l limpa o erro e volta ao nome: err=%v foco=%d", h.m.formErr, h.m.focus)
	}
	h.mustNotContain("inválido")
	h.mustContain("ctrl+l limpar")

	// Na edição, limpar mantém o modo edição.
	h.press(esc)
	h.press(ch('e'), tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	if h.m.editing == nil || h.m.fields[fName].Value() != "" {
		t.Fatal("ctrl+l na edição limpa os campos e continua editando")
	}
}

func TestCtrlLParaAPrevia(t *testing.T) {
	t.Parallel()
	h := newHarness(t, feramusicas.NewMemoryCatalog())
	h.press(ch('a'))
	fillForm(h, "x", "Rock", "https://youtu.be/fJ9rUzIMcZQ")
	h.press(ctrlP)
	h.waitPreview()
	h.press(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	if h.m.deps.Player.Playing() {
		t.Fatal("limpar deveria parar a prévia")
	}
}

func TestConfirmacaoCabeEm80x24(t *testing.T) {
	t.Parallel()
	h := seeded(t)
	h.press(ch('a'))
	fillForm(h, strings.Repeat("Nome comprido ", 8), "Categoria bem comprida mesmo", "https://youtu.be/aaaaaaaaaaa")
	h.press(tab)
	h.typeText("11:59:59.5")
	h.press(enter)
	v := h.m.View()
	if n := strings.Count(v, "\n") + 1; n > 24 {
		t.Fatalf("%d linhas:\n%s", n, ansi.Strip(v))
	}
	for _, l := range strings.Split(v, "\n") {
		if w := lipgloss.Width(l); w > 80 {
			t.Fatalf("linha com %d colunas: %q", w, ansi.Strip(l))
		}
	}
}
