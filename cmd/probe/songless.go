package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"fera-games/internal/audio"
	"fera-games/internal/games/songless"
	"fera-games/internal/httpx"
)

func readerOf(b []byte) *bytes.Reader { return bytes.NewReader(b) }

var songlessClient = songless.NewClient(httpx.New(60*time.Second), nil)

// probeSonglessToday raspa a página e mostra data + categorias + challenge.
func probeSonglessToday(ctx context.Context) error {
	d, err := songlessClient.Daily(ctx, time.Now())
	if err != nil {
		return err
	}
	n, _ := songless.ChallengeNumber(d.Date)
	fmt.Printf("Songless #%d (%s)\n", n, d.Date)
	for _, c := range d.Categories {
		fmt.Printf("  cat %d %s: %s\n", c.ID, c.Name, c.FileURL)
	}
	fmt.Printf("timesteps: %v\n", songless.Timesteps)
	return nil
}

// probeSonglessSearch busca o texto na API.
func probeSonglessSearch(ctx context.Context, q string) error {
	entries, err := songlessClient.Search(ctx, q)
	if err != nil {
		return err
	}
	fmt.Printf("%d resultados para %q:\n", len(entries), q)
	for i, e := range entries {
		if i >= 10 {
			break
		}
		fmt.Printf("  %s | %s - %s\n", e.ID, e.Artist, e.Track)
	}
	return nil
}

// probeSonglessGuess chuta skip até revelar a resposta da categoria.
func probeSonglessGuess(ctx context.Context, cat int) error {
	d, err := songlessClient.Daily(ctx, time.Now())
	if err != nil {
		return err
	}
	fmt.Printf("data do servidor: %s (categoria %d)\n", d.Date, cat)
	for round := 0; round < songless.MaxGuesses; round++ {
		out, ans, err := songlessClient.Guess(ctx, songless.Guess{
			Date: d.Date, CategoryID: cat, RoundIndex: round, SongID: nil,
		})
		if err != nil {
			return err
		}
		fmt.Printf("round %d: %s", round, out)
		if ans != nil {
			fmt.Printf(" → %s - %s (sc=%s)", ans.Artist, ans.Track, ans.SC)
		}
		fmt.Println()
		if out == songless.OutcomeCorrect {
			break
		}
	}
	return nil
}

// probeSonglessPlay baixa o .aac da categoria e toca até o limite do round.
func probeSonglessPlay(ctx context.Context, cat, round int) error {
	d, err := songlessClient.Daily(ctx, time.Now())
	if err != nil {
		return err
	}
	c, ok := d.Category(cat)
	if !ok {
		return fmt.Errorf("categoria %d não existe", cat)
	}
	rc, err := songlessClient.Audio(ctx, c)
	if err != nil {
		return err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return err
	}
	fmt.Printf("baixados %d bytes (%s) de %s\n", len(data), Sniff(data), c.FileURL)
	track, err := audio.DecodeAAC(readerOf(data))
	if err != nil {
		return err
	}
	limit := songless.LimitFor(round)
	fmt.Printf("duração %s, limite do round %d = %s\n", track.Duration().Round(time.Millisecond), round, limit)
	p, err := audio.Open()
	if err != nil {
		return err
	}
	defer p.Close()
	p.Restart(track)
	p.Play()
	fmt.Printf("tocando até %s…\n", limit)
	deadline := time.Now().Add(limit + 500*time.Millisecond)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			if p.Position() >= limit || time.Now().After(deadline) || !p.Playing() {
				// Deixa o fim da faixa aparecer antes de sair.
				if !p.Playing() {
					fmt.Println("fim")
					return nil
				}
				p.Pause()
				fmt.Println("limite atingido, pausado em", p.Position().Round(time.Millisecond))
				return nil
			}
		}
	}
}
