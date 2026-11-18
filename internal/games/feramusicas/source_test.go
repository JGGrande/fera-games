package feramusicas

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeTools cria "yt-dlp" e "ffmpeg" falsos (scripts sh) numa pasta temporária.
// Cada um registra os argumentos em <nome>.args e se comporta conforme o script.
func fakeTools(t *testing.T, ytdlp, ffmpeg string) *YTDLP {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("scripts sh")
	}
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + p + ".args'\n" + body + "\n"
		if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	return &YTDLP{YTDLPPath: write("yt-dlp", ytdlp), FFmpegPath: write("ffmpeg", ffmpeg)}
}

func args(t *testing.T, tool string) []string {
	t.Helper()
	b, err := os.ReadFile(tool + ".args")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

var song = Song{ID: "s1", Name: "x", Category: "Rock", VideoID: "fJ9rUzIMcZQ"}

func TestYTDLPBaixaSoOTrechoNecessario(t *testing.T) {
	y := fakeTools(t,
		`echo 'https://stream.example/audio.m4a'`,
		`printf 'aaaabbbb'`) // 2 amostras estéreo
	rc, err := y.Fetch(context.Background(), song, AudioWindow)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	if string(got) != "aaaabbbb" {
		t.Fatalf("PCM = %q", got)
	}

	yt := args(t, y.YTDLPPath)
	if yt[len(yt)-1] != "https://www.youtube.com/watch?v=fJ9rUzIMcZQ" {
		t.Fatalf("yt-dlp recebeu a URL %q; quero a canônica montada do VideoID", yt[len(yt)-1])
	}
	for _, want := range []string{"-g", "--no-playlist"} {
		if !contains(yt, want) {
			t.Errorf("yt-dlp sem %s: %v", want, yt)
		}
	}

	ff := args(t, y.FFmpegPath)
	// -t precisa vir antes de -i para limitar a leitura da entrada.
	if ti, ii := index(ff, "-t"), index(ff, "-i"); ti < 0 || ii < 0 || ti > ii || ff[ti+1] != "15.50" {
		t.Fatalf("ffmpeg deveria limitar a entrada a 15.50 s antes do -i: %v", ff)
	}
	if ff[index(ff, "-i")+1] != "https://stream.example/audio.m4a" {
		t.Fatalf("ffmpeg não leu a URL do yt-dlp: %v", ff)
	}
	for _, want := range []string{"-vn", "s16le", "44100", "pipe:1"} {
		if !contains(ff, want) {
			t.Errorf("ffmpeg sem %s: %v", want, ff)
		}
	}
}

func TestYTDLPCacheEmMemoria(t *testing.T) {
	y := fakeTools(t, `echo https://stream.example/a`, `printf 'aaaa'`)
	for i := 0; i < 2; i++ {
		rc, err := y.Fetch(context.Background(), song, AudioWindow)
		if err != nil {
			t.Fatal(err)
		}
		_ = rc.Close()
		if i == 0 {
			// Quebra as ferramentas: o 2º Fetch só funciona se vier do cache.
			y.YTDLPPath, y.FFmpegPath = "/nao/existe", "/nao/existe"
		}
	}
}

func TestYTDLPPegaAPrimeiraURL(t *testing.T) {
	y := fakeTools(t, `printf '\nhttps://video.example/v\nhttps://audio.example/a\n'`, `printf 'aaaa'`)
	if _, err := y.Fetch(context.Background(), song, time.Second); err != nil {
		t.Fatal(err)
	}
	ff := args(t, y.FFmpegPath)
	if ff[index(ff, "-i")+1] != "https://video.example/v" {
		t.Fatalf("ffmpeg: %v", ff)
	}
}

func TestYTDLPErrosTraduzidos(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
		want   error
	}{
		{"indisponível", "ERROR: [youtube] x: Video unavailable", ErrVideoUnavailable},
		{"privado", "ERROR: [youtube] x: Private video. Sign in if you've been granted access", ErrVideoUnavailable},
		{"região", "ERROR: The uploader has not made this video available in your country", ErrVideoUnavailable},
		{"idade", "ERROR: [youtube] x: Sign in to confirm your age", ErrAgeRestricted},
		{"bot", "ERROR: [youtube] x: Sign in to confirm you’re not a bot", ErrBotCheck},
	}
	for _, tc := range cases {
		y := fakeTools(t, "cat >&2 <<'EOT'\n"+tc.stderr+"\nEOT\nexit 1", `printf 'aaaa'`)
		_, err := y.Fetch(context.Background(), song, time.Second)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, quero %v", tc.name, err, tc.want)
		}
	}

	// Falha de extração/formato não é "vídeo indisponível": a música está certa,
	// o yt-dlp é que precisa de atualização. A mensagem crua segue para a tela.
	for _, stderr := range []string{
		"ERROR: [youtube] fJ9rUzIMcZQ: Requested format is not available. Use --list-formats for a list of available formats",
		"ERROR: [youtube] fJ9rUzIMcZQ: nsig extraction failed: Some formats may be missing",
	} {
		y := fakeTools(t, "cat >&2 <<'EOT'\n"+stderr+"\nEOT\nexit 1", ``)
		_, err := y.Fetch(context.Background(), song, time.Second)
		if errors.Is(err, ErrVideoUnavailable) || err == nil || !strings.Contains(err.Error(), "ERROR: [youtube]") {
			t.Errorf("stderr %q virou %v; quero a mensagem crua, não ErrVideoUnavailable", stderr, err)
		}
	}
	for _, stderr := range []string{
		"ERROR: [youtube] x: This video is not available",
		"ERROR: [youtube] x: This video is unavailable",
		"ERROR: [youtube] x: This video has been removed by the uploader",
		"ERROR: [youtube] x: Join this channel to get access to members-only content like this video, and other exclusive perks. This video is available to this channel's members on level: X",
	} {
		y := fakeTools(t, "cat >&2 <<'EOT'\n"+stderr+"\nEOT\nexit 1", ``)
		if _, err := y.Fetch(context.Background(), song, time.Second); !errors.Is(err, ErrVideoUnavailable) {
			t.Errorf("stderr %q virou %v; quero ErrVideoUnavailable", stderr, err)
		}
	}

	y := fakeTools(t, "echo 'ERROR: algo estranho' >&2; exit 1", ``)
	if _, err := y.Fetch(context.Background(), song, time.Second); err == nil || !strings.Contains(err.Error(), "algo estranho") {
		t.Errorf("erro desconhecido deveria carregar a última linha do stderr: %v", err)
	}

	y = fakeTools(t, `echo https://s`, `echo 'Server returned 403 Forbidden' >&2; exit 1`)
	if _, err := y.Fetch(context.Background(), song, time.Second); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("falha do ffmpeg: %v", err)
	}

	y = fakeTools(t, `echo https://s`, `true`) // ffmpeg sem saída
	if _, err := y.Fetch(context.Background(), song, time.Second); err == nil {
		t.Error("áudio vazio deveria ser erro")
	}

	y = fakeTools(t, `true`, `true`) // yt-dlp sem URL
	if _, err := y.Fetch(context.Background(), song, time.Second); err == nil {
		t.Error("yt-dlp sem URL deveria ser erro")
	}
}

func TestYTDLPBinarioAusente(t *testing.T) {
	y := &YTDLP{YTDLPPath: "/nao/existe/yt-dlp", FFmpegPath: "/nao/existe/ffmpeg"}
	_, err := y.Fetch(context.Background(), song, time.Second)
	if !errors.Is(err, ErrMissingTools) {
		t.Fatalf("err = %v, quero ErrMissingTools", err)
	}
}

func TestYTDLPCancelamentoMataOProcesso(t *testing.T) {
	y := fakeTools(t, `sleep 30`, ``)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := y.Fetch(ctx, song, time.Second)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, quero DeadlineExceeded", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("o processo não foi morto no timeout (%v)", time.Since(start))
	}
}

func TestYTDLPRejeitaVideoIDInvalido(t *testing.T) {
	y := fakeTools(t, `echo https://s`, `printf 'aaaa'`)
	for _, id := range []string{"", "curto", "--exec=rm -rf /", "fJ9rUzIMcZQ;ls"} {
		if _, err := y.Fetch(context.Background(), Song{VideoID: id}, time.Second); !errors.Is(err, ErrInvalid) {
			t.Errorf("VideoID %q: err = %v, quero ErrInvalid", id, err)
		}
	}
}

func TestNewYTDLPSemFerramentas(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := NewYTDLP()
	if !errors.Is(err, ErrMissingTools) || !strings.Contains(err.Error(), "yt-dlp") || !strings.Contains(err.Error(), "ffmpeg") {
		t.Fatalf("err = %v", err)
	}
}

func TestUnavailableSource(t *testing.T) {
	boom := errors.New("sem ferramentas")
	if _, err := UnavailableSource(boom).Fetch(context.Background(), song, time.Second); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func contains(s []string, v string) bool { return index(s, v) >= 0 }

func index(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func TestYTDLPComInicioPulaDiretoParaOTrecho(t *testing.T) {
	y := fakeTools(t, `echo https://stream.example/a`, `printf 'aaaa'`)
	s := song
	s.Start = 90250 * time.Millisecond
	if _, err := y.Fetch(context.Background(), s, AudioWindow); err != nil {
		t.Fatal(err)
	}
	ff := args(t, y.FFmpegPath)
	ss, ti, ii := index(ff, "-ss"), index(ff, "-t"), index(ff, "-i")
	// -ss antes de -i: busca na entrada (pula sem baixar o começo).
	if ss < 0 || ss > ii || ti > ii || ff[ss+1] != "90.250" || ff[ti+1] != "15.50" {
		t.Fatalf("ffmpeg deveria receber -ss 90.250 e -t 15.50 antes do -i: %v", ff)
	}

	// Sem início, nada de -ss.
	y2 := fakeTools(t, `echo https://stream.example/a`, `printf 'aaaa'`)
	if _, err := y2.Fetch(context.Background(), song, AudioWindow); err != nil {
		t.Fatal(err)
	}
	if index(args(t, y2.FFmpegPath), "-ss") >= 0 {
		t.Fatalf("sem início não deveria haver -ss: %v", args(t, y2.FFmpegPath))
	}
}

func TestYTDLPCacheSeparaPorInicio(t *testing.T) {
	dir := t.TempDir()
	counter := dir + "/n"
	// Cada chamada do ffmpeg devolve um conteúdo diferente.
	y := fakeTools(t, `echo https://s`, `echo x >> '`+counter+`'; printf "%04d" "$(wc -l < '`+counter+`')"`)
	a := song
	b := song
	b.Start = 30 * time.Second
	read := func(s Song) string {
		rc, err := y.Fetch(context.Background(), s, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		return string(data)
	}
	first, other, again := read(a), read(b), read(a)
	if first == other {
		t.Fatalf("mesmo vídeo com outro início devolveu o trecho antigo do cache: %q", other)
	}
	if again != first {
		t.Fatalf("mesmo vídeo e mesmo início deveriam vir do cache: %q vs %q", again, first)
	}
}

func TestYTDLPInicioDepoisDoFim(t *testing.T) {
	y := fakeTools(t, `echo https://s`, `true`) // ffmpeg sai sem áudio
	s := song
	s.Start = 10 * time.Minute
	_, err := y.Fetch(context.Background(), s, time.Second)
	if !errors.Is(err, ErrStartPastEnd) || !strings.Contains(err.Error(), "10:00") {
		t.Fatalf("err = %v; quero ErrStartPastEnd citando o início", err)
	}
	if _, err := y.Fetch(context.Background(), song, time.Second); !errors.Is(err, ErrNoAudio) || errors.Is(err, ErrStartPastEnd) {
		t.Fatalf("sem início: err = %v; quero ErrNoAudio", err)
	}
	s.Start = -time.Second
	if _, err := y.Fetch(context.Background(), s, time.Second); !errors.Is(err, ErrInvalid) {
		t.Fatalf("início negativo: %v", err)
	}
}
