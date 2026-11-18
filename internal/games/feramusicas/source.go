package feramusicas

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AudioSource entrega o começo de uma música como PCM s16le, estéreo, 44,1 kHz
// (o que audio.DecodePCM lê), com no máximo `max` de duração.
type AudioSource interface {
	Fetch(ctx context.Context, s Song, max time.Duration) (io.ReadCloser, error)
}

// Erros de download, para a TUI explicar o que fazer.
var (
	// ErrMissingTools: yt-dlp e/ou ffmpeg não estão instalados.
	ErrMissingTools = errors.New("faltam ferramentas de download")
	// ErrVideoUnavailable: vídeo removido, privado ou bloqueado na região.
	ErrVideoUnavailable = errors.New("vídeo indisponível")
	// ErrAgeRestricted: o YouTube exige login para esse vídeo.
	ErrAgeRestricted = errors.New("vídeo com restrição de idade")
	// ErrBotCheck: o YouTube pediu verificação anti-bot.
	ErrBotCheck = errors.New("o YouTube pediu verificação anti-bot")
	// ErrNoAudio: o ffmpeg terminou sem devolver nenhuma amostra.
	ErrNoAudio = errors.New("o vídeo não devolveu áudio")
	// ErrStartPastEnd: com início cadastrado, não veio áudio a partir dele
	// (o início provavelmente passa do fim do vídeo).
	ErrStartPastEnd = errors.New("não há áudio a partir do início cadastrado")
)

// InstallHint diz o que instalar quando faltam ferramentas.
const InstallHint = "instale o yt-dlp e o ffmpeg e deixe os dois no PATH (veja o README)"

// YTDLP baixa o áudio direto do YouTube no cliente: o yt-dlp resolve a URL do
// stream e o ffmpeg lê só os primeiros segundos, já convertidos para PCM.
// Nada vai para o disco; o resultado fica em memória até o programa fechar,
// para trocar de categoria e voltar não baixar de novo.
type YTDLP struct {
	YTDLPPath  string
	FFmpegPath string

	mu    sync.Mutex
	cache map[string][]byte // cacheKey (vídeo + início) → PCM
}

// cacheKey inclui o início: mudar o início (remover e cadastrar de novo) tem
// de baixar outro trecho, não reaproveitar o antigo.
func cacheKey(s Song) string { return s.VideoID + "@" + strconv.FormatInt(s.Start.Milliseconds(), 10) }

var _ AudioSource = (*YTDLP)(nil)

// NewYTDLP procura yt-dlp e ffmpeg no PATH. Se faltar algum, devolve um erro
// que satisfaz errors.Is(err, ErrMissingTools) dizendo quais.
func NewYTDLP() (*YTDLP, error) {
	var missing []string
	paths := map[string]string{}
	for _, name := range []string{"yt-dlp", "ffmpeg"} {
		p, err := exec.LookPath(name)
		if err != nil {
			missing = append(missing, name)
			continue
		}
		paths[name] = p
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrMissingTools, strings.Join(missing, ", "))
	}
	return &YTDLP{YTDLPPath: paths["yt-dlp"], FFmpegPath: paths["ffmpeg"]}, nil
}

// Fetch baixa `max` segundos da música a partir de s.Start.
func (y *YTDLP) Fetch(ctx context.Context, s Song, max time.Duration) (io.ReadCloser, error) {
	if !videoIDRe.MatchString(s.VideoID) {
		return nil, &ValidationError{FieldURL, "música sem ID de vídeo válido"}
	}
	if s.Start < 0 {
		return nil, &ValidationError{FieldStart, "o início não pode ser negativo"}
	}
	key := cacheKey(s)
	y.mu.Lock()
	pcm, ok := y.cache[key]
	y.mu.Unlock()
	if ok {
		return io.NopCloser(bytes.NewReader(pcm)), nil
	}

	streamURL, err := y.streamURL(ctx, s.VideoID)
	if err != nil {
		return nil, err
	}
	pcm, err = y.decode(ctx, streamURL, s.Start, max)
	if errors.Is(err, ErrNoAudio) && s.Start > 0 {
		return nil, fmt.Errorf("%w (%s): confira se o vídeo é mais longo que isso", ErrStartPastEnd, FormatStart(s.Start))
	}
	if err != nil {
		return nil, err
	}

	y.mu.Lock()
	if y.cache == nil {
		y.cache = map[string][]byte{}
	}
	y.cache[key] = pcm
	y.mu.Unlock()
	return io.NopCloser(bytes.NewReader(pcm)), nil
}

// streamURL pergunta ao yt-dlp a URL direta do melhor áudio (sem baixar).
func (y *YTDLP) streamURL(ctx context.Context, videoID string) (string, error) {
	out, err := y.run(ctx, y.YTDLPPath,
		"--no-playlist", "--no-warnings", "-f", "bestaudio/best", "-g", WatchURL(videoID))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line, nil // se vierem várias (vídeo+áudio), a primeira basta: -vn descarta o vídeo
		}
	}
	return "", errors.New("o yt-dlp não devolveu nenhuma URL de áudio")
}

// decode pede ao ffmpeg `max` segundos a partir de start, em PCM s16le 44,1 kHz
// estéreo. -ss e -t antes de -i são opções de entrada: o ffmpeg pula direto
// para o início (com pedido de range ao servidor) e para de ler no fim do
// trecho, então nem o que vem antes nem o resto do vídeo são baixados.
func (y *YTDLP) decode(ctx context.Context, streamURL string, start, max time.Duration) ([]byte, error) {
	args := []string{"-nostdin", "-loglevel", "error"}
	if start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(start.Seconds(), 'f', 3, 64))
	}
	args = append(args, "-t", strconv.FormatFloat(max.Seconds(), 'f', 2, 64), "-i", streamURL,
		"-vn", "-f", "s16le", "-ar", "44100", "-ac", "2", "pipe:1")
	out, err := y.run(ctx, y.FFmpegPath, args...)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, ErrNoAudio
	}
	return out, nil
}

// run executa um programa e devolve o stdout; em caso de falha, traduz o stderr.
func (y *YTDLP) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	// Se o processo deixar um filho vivo segurando o pipe (o yt-dlp faz isso
	// ao usar um runtime de JS), o Wait só volta depois deste prazo, e não
	// quando o filho resolver terminar.
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, explain(ctx, name, err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// explain troca a falha crua de um processo por um erro que a TUI entende.
func explain(ctx context.Context, name string, err error, stderr string) error {
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrMissingTools, name)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr // timeout/cancelamento valem mais que "signal: killed"
	}
	low := strings.ToLower(stderr)
	switch {
	case strings.Contains(low, "confirm your age") || strings.Contains(low, "age-restricted"):
		return ErrAgeRestricted
	case strings.Contains(low, "not a bot") || strings.Contains(low, "sign in to confirm"):
		return fmt.Errorf("%w; atualize o yt-dlp (yt-dlp -U) ou tente mais tarde", ErrBotCheck)
	case isUnavailable(low):
		return ErrVideoUnavailable
	}
	if line := lastLine(stderr); line != "" {
		return fmt.Errorf("%s falhou: %s", name, line)
	}
	return fmt.Errorf("%s falhou: %w", name, err)
}

// unavailablePhrases são mensagens do yt-dlp que dizem que o vídeo em si não
// existe para nós (e a música deve sair do catálogo). Precisam ser frases
// inteiras: "not available" sozinho também casa com "Requested format is not
// available", falha passageira de extração que se resolve com yt-dlp -U.
var unavailablePhrases = []string{
	"video unavailable",
	"this video is unavailable",
	"this video is not available",
	"private video",
	"has been removed",
	"has been terminated", // conta do canal encerrada
	"in your country",
	"available to this channel's members",
}

func isUnavailable(lowStderr string) bool {
	for _, p := range unavailablePhrases {
		if strings.Contains(lowStderr, p) {
			return true
		}
	}
	return false
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// unavailable é a fonte usada quando as ferramentas faltam: o jogo abre, o
// cadastro funciona, e cada tentativa de baixar devolve o motivo.
type unavailable struct{ err error }

func (u unavailable) Fetch(context.Context, Song, time.Duration) (io.ReadCloser, error) {
	return nil, u.err
}

// UnavailableSource devolve uma AudioSource que sempre falha com err.
func UnavailableSource(err error) AudioSource { return unavailable{err} }
