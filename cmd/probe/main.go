// Command probe valida na prática o que a Fase 0 descobriu:
//   - mostra o número do puzzle do dia (fórmula a confirmar com o payload real);
//   - baixa só o começo de uma URL (puzzle JSON ou faixa de áudio) e detecta o formato;
//   - com a tag de build "audio", toca a faixa (oto v3 + beep).
//
// Uso:
//
//	go run ./cmd/probe                                  # número do puzzle de hoje
//	go run ./cmd/probe -url https://.../puzzle.json     # status, headers e formato
//	go run -tags audio ./cmd/probe -url https://.../drums.mp3 -play
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"fera-games/internal/games/bandle"
)

const userAgent = "FeraGames/0.0 (uso pessoal; cliente de terminal)"

// playFunc é preenchida em play_audio.go quando compilado com -tags audio.
var playFunc func(ctx context.Context, url string) error

func main() {
	rawURL := flag.String("url", "", "URL a inspecionar (JSON do puzzle ou faixa de áudio)")
	play := flag.Bool("play", false, "tocar a URL como áudio (requer -tags audio)")
	limit := flag.Int64("bytes", 64<<10, "máximo de bytes lidos na inspeção")
	flag.Parse()

	now := time.Now()
	n := bandle.PuzzleNumber(now)
	fmt.Printf("Hoje (%s, fuso local): Bandle #%d\n", now.Format("2006-01-02"), n)
	if *rawURL == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if *play {
		if playFunc == nil {
			log.Fatal("binário compilado sem áudio: use `go run -tags audio ./cmd/probe ...`")
		}
		if err := playFunc(context.Background(), *rawURL); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := inspect(ctx, *rawURL, *limit); err != nil {
		log.Fatal(err)
	}
}

func inspect(ctx context.Context, url string, limit int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", limit-1))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	fmt.Printf("GET %s\nstatus: %s\n", url, resp.Status)
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges",
		"Cache-Control", "ETag", "Last-Modified", "Access-Control-Allow-Origin", "Server"} {
		if v := resp.Header.Get(h); v != "" {
			fmt.Printf("%s: %s\n", h, v)
		}
	}

	head, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return err
	}
	fmt.Printf("lidos: %d bytes\nformato detectado: %s\n", len(head), Sniff(head))
	if bytes.HasPrefix(bytes.TrimSpace(head), []byte("{")) || bytes.HasPrefix(bytes.TrimSpace(head), []byte("[")) {
		preview := string(head)
		if len(preview) > 400 {
			preview = preview[:400] + "…"
		}
		fmt.Printf("prévia (cuidado com spoiler):\n%s\n", preview)
	}
	return nil
}

// Sniff identifica o formato pelos magic bytes.
func Sniff(b []byte) string {
	switch {
	case len(b) == 0:
		return "vazio"
	case bytes.HasPrefix(b, []byte("ID3")):
		return "mp3 (com tag ID3)"
	case len(b) > 1 && b[0] == 0xFF && b[1]&0xE0 == 0xE0 && b[1]&0x06 == 0:
		return "aac (ADTS)"
	case len(b) > 1 && b[0] == 0xFF && b[1]&0xE0 == 0xE0:
		return "mp3 (frame MPEG)"
	case bytes.HasPrefix(b, []byte("OggS")):
		if bytes.Contains(b[:min(len(b), 64)], []byte("OpusHead")) {
			return "opus (ogg)"
		}
		return "ogg (vorbis)"
	case len(b) > 11 && string(b[4:8]) == "ftyp":
		return "mp4/m4a (ftyp " + strings.TrimSpace(string(b[8:12])) + ")"
	case bytes.HasPrefix(b, []byte("RIFF")) && len(b) > 11 && string(b[8:12]) == "WAVE":
		return "wav"
	case bytes.HasPrefix(b, []byte("fLaC")):
		return "flac"
	case bytes.HasPrefix(b, []byte{0x1A, 0x45, 0xDF, 0xA3}):
		return "webm/matroska"
	}
	s := bytes.TrimSpace(b)
	if len(s) > 0 && (s[0] == '{' || s[0] == '[') {
		return "json"
	}
	if http.DetectContentType(b) != "application/octet-stream" {
		return http.DetectContentType(b)
	}
	return "desconhecido"
}

func init() {
	log.SetFlags(0)
	log.SetOutput(os.Stderr)
}
