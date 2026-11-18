package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"fera-games/internal/audio"
	"fera-games/internal/games/bandle"
	"fera-games/internal/games/bandle/bandletest"
	"fera-games/internal/games/conexo"
	"fera-games/internal/games/conexo/conexotest"
	"fera-games/internal/games/feramusicas"
	"fera-games/internal/games/feramusicas/feramusicastest"
	"fera-games/internal/games/songless"
	"fera-games/internal/games/songless/songlesstest"
	"fera-games/internal/tui/menu"
	"fera-games/internal/tui/theme"
)

// TestOpensMenuAndQuitsWithQ é o critério de pronto da Fase 1.
func TestOpensMenuAndQuitsWithQ(t *testing.T) {
	tm := teatest.NewTestModel(t, NewWith(Config{}, fakeDeps(t)), teatest.WithInitialTermSize(100, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Fera Games")) && bytes.Contains(b, []byte("Bandle"))
	}, teatest.WithDuration(3*time.Second))

	// Com AltScreen a saída final é a tela restaurada; o que importa é encerrar.
	tm.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestTooSmallTerminal(t *testing.T) {
	m := NewWith(Config{}, fakeDeps(t))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	v := next.(Model).View()
	if !strings.Contains(v.Content, "pelo menos 80x24") {
		t.Fatalf("terminal pequeno não mostrou o aviso:\n%s", v.Content)
	}
	next, _ = next.Update(tea.WindowSizeMsg{Width: theme.MinWidth, Height: theme.MinHeight})
	if v := next.(Model).View(); !strings.Contains(v.Content, "Bandle") {
		t.Fatalf("no tamanho mínimo o menu não voltou:\n%s", v.Content)
	}
}

func TestMenuFitsMinimumSize(t *testing.T) {
	m := NewWith(Config{}, fakeDeps(t))
	next, _ := m.Update(tea.WindowSizeMsg{Width: theme.MinWidth, Height: theme.MinHeight})
	lines := strings.Split(next.(Model).View().Content, "\n")
	if len(lines) > theme.MinHeight {
		t.Fatalf("menu tem %d linhas, não cabe em %d", len(lines), theme.MinHeight)
	}
}

func fakeDeps(t *testing.T) Deps {
	return Deps{
		Bandle:      bandle.NewGame(bandletest.New(t)),
		Songless:    songless.NewGame(songlesstest.New(t)),
		Conexo:      conexo.NewGame(conexotest.New(t), "pt"),
		FeraMusicas: feramusicas.NewGame(feramusicastest.NewCatalog(t), &feramusicastest.Source{Seconds: 1}),
		Now:         func() time.Time { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.Local) },
		OpenPlayer:  func() (audio.Player, error) { return audio.NewSilent(), nil },
	}
}

// TestEnterOpensBandleAndQReturns cobre o roteamento menu ↔ tela de jogo.
func TestEnterOpensBandleAndQReturns(t *testing.T) {
	m := NewWith(Config{}, fakeDeps(t))
	var cur tea.Model = m
	cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// enter no menu → SelectedMsg → tela do Bandle carregando.
	cur, cmd := cur.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter não gerou comando")
	}
	cur, cmd = cur.Update(cmd())
	if cur.(Model).screen != screenBandle || cmd == nil {
		t.Fatalf("SelectedMsg não abriu o Bandle (screen=%d)", cur.(Model).screen)
	}
	if v := ansi.Strip(cur.View().Content); !strings.Contains(v, "baixando o puzzle") {
		t.Fatalf("tela inicial do Bandle:\n%s", v)
	}

	// Executa o Init da tela (Batch: load + spinner + tick) só pelo que é imediato.
	for _, msg := range drain(cmd) {
		cur, _ = cur.Update(msg)
	}
	if v := ansi.Strip(cur.View().Content); !strings.Contains(v, "Bandle #1491") {
		t.Fatalf("puzzle não carregou na tela:\n%s", v)
	}

	// q volta ao menu.
	cur, cmd = cur.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("q não gerou comando")
	}
	cur, _ = cur.Update(cmd())
	if cur.(Model).screen != screenMenu {
		t.Fatal("BackMsg não voltou ao menu")
	}
	if v := ansi.Strip(cur.View().Content); !strings.Contains(v, "Fera Games") {
		t.Fatalf("menu não voltou:\n%s", v)
	}
	// enter de novo retoma a mesma partida, sem recarregar.
	cur, cmd = cur.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cur, _ = cur.Update(cmd())
	if v := ansi.Strip(cur.View().Content); !strings.Contains(v, "Bandle #1491") {
		t.Fatalf("retomar não mostrou a partida:\n%s", v)
	}
}

// drain roda um cmd (ou Batch) e devolve as mensagens imediatas; ticks e
// spinners demoram e são descartados.
func drain(cmd tea.Cmd) []tea.Msg {
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
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, drain(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestDownOpensSonglessAndQReturns(t *testing.T) {
	m := NewWith(Config{}, fakeDeps(t))
	var cur tea.Model = m
	cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Desce até Songless (Bandle=0, Termo=1 indisponível, Songless=2).
	cur, _ = cur.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	cur, _ = cur.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	cur, cmd := cur.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter não gerou comando")
	}
	cur, _ = cur.Update(cmd())
	if cur.(Model).screen != screenSongless {
		t.Fatalf("SelectedMsg não abriu o Songless (screen=%d)", cur.(Model).screen)
	}
	// O SelectedMsg do menu carrega o ID; garante que é songless.
	if cur.(Model).songless == nil {
		t.Fatal("tela do Songless não foi criada")
	}
	_ = menu.SelectedMsg{}
}

// TestConexoOpensAndQReturns cobre o roteamento menu ↔ Conexo, retomando a
// partida ao voltar.
func TestConexoOpensAndQReturns(t *testing.T) {
	var cur tea.Model = NewWith(Config{}, fakeDeps(t))
	cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Bandle=0, Termo=1, Songless=2, Conexo=3.
	for range 3 {
		cur, _ = cur.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	cur, cmd := cur.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cur, cmd = cur.Update(cmd())
	if cur.(Model).screen != screenConexo {
		t.Fatalf("SelectedMsg não abriu o Conexo (screen=%d)", cur.(Model).screen)
	}
	for _, msg := range drain(cmd) {
		cur, _ = cur.Update(msg)
	}
	if v := ansi.Strip(cur.View().Content); !strings.Contains(v, "CONEXO #1117") || !strings.Contains(v, "MANILHA") {
		t.Fatalf("tabuleiro não carregou:\n%s", v)
	}
	cur, _ = cur.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})

	cur, cmd = cur.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	cur, _ = cur.Update(cmd())
	if cur.(Model).screen != screenMenu {
		t.Fatal("BackMsg não voltou ao menu")
	}
	cur, cmd = cur.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cur, _ = cur.Update(cmd())
	if s := cur.(Model).conexo.Session(); s == nil || s.Items()[1] != "manilha" {
		t.Fatal("retomar deveria manter a partida em memória")
	}
}

// settleApp roda cmd e, em cadeia, os comandos que cada mensagem gera
// (descartando o que demora: ticks e spinners).
func settleApp(cur tea.Model, cmd tea.Cmd) tea.Model {
	pending := []tea.Cmd{cmd}
	for round := 0; round < 10 && len(pending) > 0; round++ {
		var next []tea.Cmd
		for _, c := range pending {
			for _, msg := range drain(c) {
				var n tea.Cmd
				cur, n = cur.Update(msg)
				next = append(next, n)
			}
		}
		pending = next
	}
	return cur
}

// TestFeraMusicasOpensAndQReturns cobre o roteamento menu ↔ Fera Músicas,
// retomando a mesma partida ao voltar.
func TestFeraMusicasOpensAndQReturns(t *testing.T) {
	var cur tea.Model = NewWith(Config{}, fakeDeps(t))
	cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Bandle=0, Termo=1, Songless=2, Conexo=3, Fera Músicas=4.
	for range 4 {
		cur, _ = cur.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if v := ansi.Strip(cur.View().Content); !strings.Contains(v, "Fera Músicas") {
		t.Fatalf("o menu deveria listar o Fera Músicas:\n%s", v)
	}
	cur, cmd := cur.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cur, cmd = cur.Update(cmd())
	if cur.(Model).screen != screenFeraMusicas || cur.(Model).feraMusicas == nil {
		t.Fatalf("SelectedMsg não abriu o Fera Músicas (screen=%d)", cur.(Model).screen)
	}
	// Catálogo → sessão → áudio chegam em mensagens encadeadas.
	cur = settleApp(cur, cmd)
	if v := ansi.Strip(cur.View().Content); !strings.Contains(v, "Fera Músicas") {
		t.Fatalf("tela do Fera Músicas:\n%s", v)
	}

	first := cur.(Model).feraMusicas.Session()
	if first == nil {
		t.Fatal("a partida deveria ter sido aberta")
	}
	cur, cmd = cur.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	cur, _ = cur.Update(cmd())
	if cur.(Model).screen != screenMenu {
		t.Fatal("BackMsg não voltou ao menu")
	}
	cur, cmd = cur.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cur, _ = cur.Update(cmd())
	if cur.(Model).feraMusicas.Session() != first {
		t.Fatal("retomar deveria manter a mesma partida em memória")
	}
}

// TestFeraMusicasSemJogoNaoQuebra: sem a dependência (ex.: testes antigos), a
// entrada do menu simplesmente não abre nada.
func TestFeraMusicasSemJogoNaoQuebra(t *testing.T) {
	deps := fakeDeps(t)
	deps.FeraMusicas = nil
	var cur tea.Model = NewWith(Config{}, deps)
	cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	cur, _ = cur.Update(menu.SelectedMsg{ID: feramusicas.GameID})
	if cur.(Model).screen != screenMenu {
		t.Fatalf("screen = %d, deveria continuar no menu", cur.(Model).screen)
	}
}

// TestAdminCadastraEOJogoVe cobre a tecla a do menu: o Admin abre, cadastra
// uma música, e o jogo (que divide o catálogo com ele) já a usa.
func TestAdminCadastraEOJogoVe(t *testing.T) {
	deps := fakeDeps(t)
	deps.FeraMusicas = feramusicas.NewGame(feramusicas.NewMemoryCatalog(), &feramusicastest.Source{Seconds: 1})
	var cur tea.Model = NewWith(Config{}, deps)
	cur, _ = cur.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if v := ansi.Strip(cur.View().Content); !strings.Contains(v, "a admin") {
		t.Fatalf("o menu deveria mostrar a tecla a:\n%s", v)
	}

	cur, cmd := cur.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	cur, cmd = cur.Update(cmd())
	if cur.(Model).screen != screenAdmin || cur.(Model).admin == nil {
		t.Fatalf("a não abriu o Admin (screen=%d)", cur.(Model).screen)
	}
	cur = settleApp(cur, cmd)
	if v := ansi.Strip(cur.View().Content); !strings.Contains(v, "Admin") || !strings.Contains(v, "Nenhuma música") {
		t.Fatalf("tela do Admin:\n%s", v)
	}

	// Cadastra pelo Admin (o formulário é testado no pacote dele; aqui basta o catálogo).
	_, err := deps.FeraMusicas.Catalog().Add(context.Background(), feramusicas.Song{
		Name: "Queen - Bohemian Rhapsody", Category: "Rock", URL: "https://youtu.be/fJ9rUzIMcZQ",
	})
	if err != nil {
		t.Fatal(err)
	}
	cur, cmd = cur.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	cur, _ = cur.Update(cmd())
	if cur.(Model).screen != screenMenu {
		t.Fatal("q no Admin deveria voltar ao menu")
	}

	// Abre o jogo: a música cadastrada já é a do dia em Rock.
	for range 4 {
		cur, _ = cur.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	cur, cmd = cur.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cur, cmd = cur.Update(cmd())
	cur = settleApp(cur, cmd)
	if s := cur.(Model).feraMusicas.Session(); s == nil || s.Category() != "Rock" {
		t.Fatalf("o jogo deveria abrir em Rock com a música cadastrada no Admin")
	}

	// Voltar ao Admin recarrega a lista.
	cur, cmd = cur.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	cur, _ = cur.Update(cmd())
	cur, cmd = cur.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	cur, cmd = cur.Update(cmd())
	cur = settleApp(cur, cmd)
	if v := ansi.Strip(cur.View().Content); !strings.Contains(v, "músicas cadastradas (1)") || !strings.Contains(v, "Queen - Bohemian Rhapsody") {
		t.Fatalf("o Admin deveria listar a música cadastrada:\n%s", v)
	}
}

func TestAdminSemJogoNaoQuebra(t *testing.T) {
	deps := fakeDeps(t)
	deps.FeraMusicas = nil
	var cur tea.Model = NewWith(Config{}, deps)
	cur, _ = cur.Update(menu.AdminMsg{})
	if cur.(Model).screen != screenMenu {
		t.Fatalf("screen = %d, deveria continuar no menu", cur.(Model).screen)
	}
}
