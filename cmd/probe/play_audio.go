//go:build audio

// Reprodução de áudio para o probe. Fica atrás da tag "audio" para que o
// resto da Fase 0 compile sem dependências externas.
//
//	go get github.com/gopxl/beep/v2@latest
//	go run -tags audio ./cmd/probe -url <faixa> -play
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/speaker"
	"github.com/gopxl/beep/v2/vorbis"
	"github.com/gopxl/beep/v2/wav"
)

const maxAudioBytes = 20 << 20

func init() {
	playFunc = play
}

func play(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAudioBytes))
	if err != nil {
		return err
	}

	rc := io.NopCloser(bytes.NewReader(data))
	var (
		stream beep.StreamSeekCloser
		format beep.Format
	)
	switch kind := Sniff(data); kind {
	case "mp3 (com tag ID3)", "mp3 (frame MPEG)":
		stream, format, err = mp3.Decode(rc)
	case "ogg (vorbis)":
		stream, format, err = vorbis.Decode(rc)
	case "wav":
		stream, format, err = wav.Decode(bytes.NewReader(data))
	default:
		return fmt.Errorf("formato %q ainda sem decoder no probe", kind)
	}
	if err != nil {
		return fmt.Errorf("decodificar: %w", err)
	}
	defer stream.Close()

	if err := speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10)); err != nil {
		return fmt.Errorf("abrir dispositivo de áudio: %w", err)
	}
	fmt.Printf("tocando %s (%d Hz, %d canais, %s)\n", url, format.SampleRate, format.NumChannels,
		format.SampleRate.D(stream.Len()).Round(time.Millisecond))

	done := make(chan struct{})
	speaker.Play(beep.Seq(stream, beep.Callback(func() { close(done) })))
	select {
	case <-done:
	case <-ctx.Done():
	}
	return nil
}
