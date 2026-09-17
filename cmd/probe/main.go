// Command probe valida na prática o protocolo do Bandle usando o mesmo
// Client da engine:
//   - mostra o número do puzzle do dia (fórmula confirmada contra o payload real);
//   - baixa um arquivo do CDN pelo fluxo real (login anônimo, URL assinada,
//     Referer) e decifra os .txt;
//   - baixa só o começo de uma URL qualquer e detecta o formato;
//   - toca a faixa ou passeia pelos estágios com internal/audio (oto v3 + beep).
//
// Uso:
//
//	go run ./cmd/probe                                  # número do puzzle de hoje
//	go run ./cmd/probe -file today                      # puzzle de hoje (campos sem spoiler)
//	go run ./cmd/probe -file /v2/guesslist/songs.txt    # qualquer arquivo do CDN
//	go run ./cmd/probe -file stem:1                     # headers da faixa 1 de hoje
//	go run ./cmd/probe -url https://.../qualquer.mp3    # status, headers e formato
//	go run ./cmd/probe -file stem:1 -play               # tocar a faixa 1 de hoje
//	go run ./cmd/probe -walk 4s                         # estágios 1..5 trocando a cada 4 s
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"fera-games/internal/games/bandle"
	"fera-games/internal/httpx"
)

// client é compartilhado para reaproveitar o token anônimo. Sem cache: o
// probe serve justamente para ver o servidor.
var client = bandle.NewClient(httpx.New(60*time.Second), nil)

func main() {
	rawURL := flag.String("url", "", "URL a inspecionar (qualquer host)")
	file := flag.String("file", "", "arquivo do CDN do Bandle: caminho (/v2/...), 'today' ou 'stem:N'")
	doPlay := flag.Bool("play", false, "tocar o resultado como áudio")
	walkEvery := flag.Duration("walk", 0, "tocar os estágios 1..5 de hoje trocando a cada intervalo (ex.: 4s)")
	spoil := flag.Bool("spoiler", false, "mostrar o payload decifrado inteiro, inclusive a resposta")
	limit := flag.Int64("bytes", 64<<10, "máximo de bytes lidos na inspeção por -url")
	flag.Parse()

	now := time.Now()
	n := bandle.PuzzleNumber(now)
	fmt.Printf("Hoje (%s, fuso local): Bandle #%d\n", now.Format("2006-01-02"), n)
	if *rawURL == "" && *file == "" && *walkEvery == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if *walkEvery > 0 {
		p, err := client.DailyPuzzle(ctx, now)
		if err != nil {
			log.Fatal(err)
		}
		if err := walk(ctx, p, *walkEvery); err != nil {
			log.Fatal(err)
		}
		return
	}

	target := *rawURL
	if *file != "" {
		path, err := resolveFile(ctx, *file, now)
		if err != nil {
			log.Fatal(err)
		}
		if !*doPlay && (strings.HasSuffix(path, ".txt") || *file == "today") {
			if err := dumpEncrypted(ctx, path, *spoil); err != nil {
				log.Fatal(err)
			}
			return
		}
		if target, err = client.SignedURL(ctx, path); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("arquivo: %s\nassinada: %s\n", path, redact(target))
	}

	if *doPlay {
		if err := play(ctx, target); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := inspect(ctx, target, *limit); err != nil {
		log.Fatal(err)
	}
}

// resolveFile traduz os atalhos do -file para um caminho do CDN.
func resolveFile(ctx context.Context, file string, now time.Time) (string, error) {
	switch {
	case file == "today":
		return bandle.PlanningFile(now), nil
	case strings.HasPrefix(file, "stem:"):
		step, err := strconv.Atoi(strings.TrimPrefix(file, "stem:"))
		if err != nil {
			return "", fmt.Errorf("-file stem:N precisa de um número: %w", err)
		}
		p, err := client.DailyPuzzle(ctx, now)
		if err != nil {
			return "", err
		}
		if step < 1 || step > p.AudioStages() {
			return "", fmt.Errorf("puzzle #%d tem áudio nos estágios 1..%d", p.ID, p.AudioStages())
		}
		return bandle.StemFile(p, step), nil
	}
	return file, nil
}

// dumpEncrypted baixa um .txt, decifra e imprime a estrutura sem a resposta,
// a menos que spoil seja verdadeiro.
func dumpEncrypted(ctx context.Context, path string, spoil bool) error {
	signed, err := client.SignedURL(ctx, path)
	if err != nil {
		return err
	}
	fmt.Printf("arquivo: %s\nassinada: %s\n", path, redact(signed))
	resp, err := client.HTTP.Get(ctx, signed, http.Header{"Referer": {bandle.Referer}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	printHeaders(resp.Header)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	plain, err := bandle.Decrypt(bandle.EncryptionKey(), string(body))
	if err != nil {
		return err
	}
	fmt.Printf("cifrado: %d bytes → decifrado: %d bytes (%s)\n", len(body), len(plain), Sniff(plain))
	if spoil {
		fmt.Println(string(plain))
		return nil
	}
	var ps []bandle.Puzzle
	if err := json.Unmarshal(plain, &ps); err == nil && len(ps) > 0 && ps[0].Path != "" {
		p := ps[0]
		fmt.Printf("puzzle #%d (%s) path=%s par=%d ano=%d gênero=%v instrumentos=%v\n",
			p.ID, p.Day, p.Path, p.Par, p.Year, p.Genre, p.Instruments)
		if want := bandle.PuzzleNumber(time.Now()); p.ID != want {
			fmt.Printf("ATENÇÃO: id do payload (%d) difere de PuzzleNumber (%d)\n", p.ID, want)
		} else {
			fmt.Println("id do payload bate com PuzzleNumber ✔")
		}
		return nil
	}
	var v any
	if err := json.Unmarshal(plain, &v); err != nil {
		fmt.Printf("prévia:\n%.400s\n", plain)
		return nil
	}
	if arr, ok := v.([]any); ok {
		fmt.Printf("array com %d itens; primeiro: %.300s\n", len(arr), first(arr))
	} else {
		fmt.Printf("objeto: %.400s\n", plain)
	}
	return nil
}

func first(arr []any) string {
	if len(arr) == 0 {
		return ""
	}
	b, _ := json.Marshal(arr[0])
	return string(b)
}

func inspect(ctx context.Context, target string, limit int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Referer", bandle.Referer)
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", limit-1))

	resp, err := client.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	fmt.Printf("GET %s\nstatus: %s\n", redact(target), resp.Status)
	printHeaders(resp.Header)

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

func printHeaders(h http.Header) {
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges",
		"Cache-Control", "ETag", "Last-Modified", "Access-Control-Allow-Origin", "Server", "X-Cache"} {
		if v := h.Get(k); v != "" {
			fmt.Printf("%s: %s\n", k, v)
		}
	}
}

// redact esconde a assinatura de uma URL do CloudFront ao imprimir.
func redact(target string) string {
	u, err := url.Parse(target)
	if err != nil {
		return target
	}
	q := u.Query()
	for _, k := range []string{"Signature", "u"} {
		if q.Has(k) {
			q.Set(k, "…")
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
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
