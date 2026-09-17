package app

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"fera-games/internal/audio"
	"fera-games/internal/games/bandle"
	"fera-games/internal/games/bandle/bandletest"
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
		Bandle:     bandle.NewGame(bandletest.New(t)),
		OpenPlayer: func() (audio.Player, error) { return audio.NewSilent(), nil },
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
