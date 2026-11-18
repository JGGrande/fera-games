package main

import (
	"context"
	"fmt"
	"time"

	"fera-games/internal/audio"
	"fera-games/internal/games/feramusicas"
)

// probeYT é a Fase 0 do Fera Músicas: valida o link, baixa só o começo do
// vídeo com yt-dlp + ffmpeg, mede o tempo e, se pedido, toca os 5 rounds.
func probeYT(ctx context.Context, link, startText string, doPlay bool) error {
	id, err := feramusicas.ParseVideoID(link)
	if err != nil {
		return err
	}
	start, err := feramusicas.ParseStart(startText)
	if err != nil {
		return err
	}
	src, err := feramusicas.NewYTDLP()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, feramusicas.InstallHint)
	}
	fmt.Printf("vídeo %s; baixando %s de áudio a partir de %s…\n", id, feramusicas.AudioWindow, start)

	began := time.Now()
	rc, err := src.Fetch(ctx, feramusicas.Song{VideoID: id, Start: start}, feramusicas.AudioWindow)
	if err != nil {
		return err
	}
	track, err := audio.DecodePCM(rc)
	rc.Close()
	if err != nil {
		return err
	}
	fmt.Printf("pronto em %s: %s de áudio (meta: < 5s)\n",
		time.Since(began).Round(10*time.Millisecond), track.Duration().Round(10*time.Millisecond))
	if !doPlay {
		return nil
	}

	p, err := audio.Open()
	if err != nil {
		return err
	}
	defer p.Close()
	for round := 1; round <= feramusicas.MaxGuesses; round++ {
		limit := feramusicas.LimitFor(round)
		// Mesmo corte da tela: a faixa termina no limite e o player para sozinho.
		p.Restart(track.Head(limit))
		fmt.Printf("round %d: tocando %s\n", round, limit)
		if err := waitEnd(ctx, p); err != nil {
			return err
		}
		fmt.Printf("         parou em %s\n", p.Position().Round(10*time.Millisecond))
		select {
		case <-time.After(700 * time.Millisecond):
		case <-ctx.Done():
			return nil
		}
	}
	return nil
}
