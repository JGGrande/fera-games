package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"fera-games/internal/audio"
	"fera-games/internal/games/bandle"
)

// play toca uma URL inteira com o Player real.
func play(ctx context.Context, url string) error {
	track, err := decodeURL(ctx, url)
	if err != nil {
		return err
	}
	p, err := audio.Open()
	if err != nil {
		return err
	}
	defer p.Close()
	p.Load(track)
	p.Play()
	fmt.Printf("tocando %s (%s)\n", redact(url), track.Duration().Round(time.Millisecond))
	return waitEnd(ctx, p)
}

// walk toca o estágio 1 e troca para 2..5 a cada `every`, mantendo a posição:
// é o teste de ouvido do critério da Fase 3 (sem estalo, em sincronia).
func walk(ctx context.Context, p bandle.Puzzle, every time.Duration) error {
	player, err := audio.Open()
	if err != nil {
		return err
	}
	defer player.Close()
	player.SetLoop(true)

	for step := 1; step <= p.AudioStages(); step++ {
		rc, err := client.Stem(ctx, p, step)
		if err != nil {
			return err
		}
		track, err := audio.Decode(rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("estágio %d: %w", step, err)
		}
		if step == 1 {
			player.Load(track)
			player.Play()
		} else {
			player.Swap(track)
		}
		fmt.Printf("estágio %d (%s) em %s, %s\n", step, p.Instruments[step-1],
			player.Position().Round(10*time.Millisecond), track.Duration().Round(time.Millisecond))
		select {
		case <-time.After(every):
		case <-ctx.Done():
			return nil
		}
	}
	fmt.Println("fim do passeio; posição final", player.Position().Round(10*time.Millisecond))
	return nil
}

func decodeURL(ctx context.Context, url string) (*audio.Track, error) {
	resp, err := client.HTTP.Get(ctx, url, http.Header{"Referer": {bandle.Referer}})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	fmt.Printf("baixados %d bytes (%s)\n", len(data), Sniff(data))
	return audio.Decode(bytes.NewReader(data))
}

func waitEnd(ctx context.Context, p audio.Player) error {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if !p.Playing() {
				fmt.Println("fim da faixa")
				return nil
			}
		}
	}
}
