// Command fera é o Fera Games: jogos diários no terminal.
//
//	fera            # abre o menu
//	fera --mute     # sem áudio
package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"fera-games/internal/app"
)

// version é preenchida pelo GoReleaser via -ldflags.
var version = "dev"

func main() {
	cfg, err := app.DefaultConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fera:", err)
		os.Exit(1)
	}
	flag.BoolVar(&cfg.Mute, "mute", false, "não tocar áudio")
	flag.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "pasta de estado e cache")
	showVersion := flag.Bool("version", false, "mostrar a versão e sair")
	flag.Parse()

	if *showVersion {
		fmt.Println("fera", version)
		return
	}

	if _, err := tea.NewProgram(app.New(cfg)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "fera:", err)
		os.Exit(1)
	}
}
