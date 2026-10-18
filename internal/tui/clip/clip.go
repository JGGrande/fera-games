// Package clip copia texto para a área de transferência.
package clip

import (
	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

// Copy usa o clipboard do sistema (xclip/xsel/wl-copy no Linux) e também
// OSC52, que cobre sessões remotas e terminais que o suportam. Qualquer um
// dos dois funcionando basta.
func Copy(text string) tea.Cmd {
	return tea.Batch(
		func() tea.Msg { _ = clipboard.WriteAll(text); return nil },
		tea.SetClipboard(text),
	)
}
