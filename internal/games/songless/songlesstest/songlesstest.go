// Package songlesstest oferece um Provider fake sobre os fixtures de
// songless/fixture, para testar engine e TUI sem rede nem dispositivo.
package songlesstest

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"fera-games/internal/games/songless"
	"fera-games/internal/games/songless/fixture"
)

// Provider serve o daily da fixture, busca fixa e chutes determinísticos.
type Provider struct {
	DailyData songless.Daily
	SearchRes []songless.Entry
	Audios    int // quantas vezes Audio foi chamado
}

var (
	answerID  = "SEMMQgZhm--14_IC7XGtZ"
	presentID = "Z-tYFxL9NVdKHg2yye5DB"
	wrongID   = "mountain-id"
)

// New carrega songless/fixture.
func New(t testing.TB) *Provider {
	t.Helper()
	d, err := songless.ParseDailyPage(fixture.PageHTML)
	if err != nil {
		t.Fatal(err)
	}
	return &Provider{
		DailyData: d,
		SearchRes: []songless.Entry{
			{ID: answerID, Artist: "Marvin Gaye", Track: "Ain't No Mountain High Enough"},
			{ID: presentID, Artist: "Marvin Gaye", Track: "Sexual Healing"},
			{ID: wrongID, Artist: "Mountain", Track: "Mississippi Queen"},
		},
	}
}

// Answer é o display da resposta (categoria All da fixture).
const Answer = "Marvin Gaye - Ain't No Mountain High Enough"

func (p *Provider) Daily(context.Context, time.Time) (songless.Daily, error) {
	return p.DailyData, nil
}

func (p *Provider) Search(_ context.Context, _ string) ([]songless.Entry, error) {
	return p.SearchRes, nil
}

func (p *Provider) Guess(_ context.Context, g songless.Guess) (songless.Outcome, *songless.Answer, error) {
	if g.SongID == nil {
		if g.RoundIndex == 4 {
			return songless.OutcomeSkipped,
				&songless.Answer{Artist: "Marvin Gaye", Track: "Ain't No Mountain High Enough", SC: "2087362767"}, nil
		}
		return songless.OutcomeSkipped, nil, nil
	}
	switch *g.SongID {
	case answerID:
		return songless.OutcomeCorrect,
			&songless.Answer{Artist: "Marvin Gaye", Track: "Ain't No Mountain High Enough", SC: "2087362767"}, nil
	case presentID:
		return songless.OutcomePresent, nil, nil
	default:
		return songless.OutcomeIncorrect, nil, nil
	}
}

func (p *Provider) Audio(_ context.Context, _ songless.Category) (io.ReadCloser, error) {
	p.Audios++
	// AAC curto real (30 frames) para decodificar rápido nos testes.
	return io.NopCloser(bytes.NewReader(fixture.ShortAAC())), nil
}

func (p *Provider) Stats(context.Context, string, int) (songless.Stats, error) {
	return songless.Stats{}, nil
}
