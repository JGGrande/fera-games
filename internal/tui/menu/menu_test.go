package menu

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"fera-games/internal/game"
)

var items = []game.Info{
	{ID: "bandle", Name: "Bandle", Description: "música", Available: true},
	{ID: "termo", Name: "Termo", Description: "palavra"},
}

func press(m Model, k string) (Model, tea.Cmd) {
	var msg tea.KeyPressMsg
	switch k {
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "up":
		msg = tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		msg = tea.KeyPressMsg{Code: tea.KeyDown}
	default:
		msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
	}
	return m.Update(msg)
}

func TestNavigationClamps(t *testing.T) {
	m := New(items)
	m, _ = press(m, "up")
	if m.Selected().ID != "bandle" {
		t.Fatalf("subir no topo mudou o cursor: %s", m.Selected().ID)
	}
	m, _ = press(m, "j")
	m, _ = press(m, "down")
	if m.Selected().ID != "termo" {
		t.Fatalf("descer além do fim mudou o cursor: %s", m.Selected().ID)
	}
}

func TestSelectAvailableEmitsMsg(t *testing.T) {
	m := New(items)
	_, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("enter em jogo disponível não gerou comando")
	}
	if got, ok := cmd().(SelectedMsg); !ok || got.ID != "bandle" {
		t.Fatalf("comando devolveu %#v, quero SelectedMsg{bandle}", cmd())
	}
}

func TestSelectUnavailableShowsNotice(t *testing.T) {
	m := New(items)
	m, _ = press(m, "down")
	m, cmd := press(m, "enter")
	if cmd != nil {
		t.Fatal("enter em jogo indisponível não deveria gerar comando")
	}
	if !strings.Contains(ansi.Strip(m.View()), "Termo ainda não está disponível") {
		t.Fatalf("aviso não apareceu:\n%s", m.View())
	}
}

func TestQuit(t *testing.T) {
	for _, k := range []string{"q"} {
		_, cmd := press(New(items), k)
		if cmd == nil {
			t.Fatalf("%q não gerou comando", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("%q não gerou tea.Quit", k)
		}
	}
	_, cmd := New(items).Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c não gerou comando")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("ctrl+c não gerou tea.Quit")
	}
}

func TestViewListsGames(t *testing.T) {
	v := ansi.Strip(New(items).View())
	for _, want := range []string{"Fera Games", "> Bandle", "Termo", "em breve", "sair"} {
		if !strings.Contains(v, want) {
			t.Errorf("view sem %q:\n%s", want, v)
		}
	}
}
