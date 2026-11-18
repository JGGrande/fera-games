// Package audio toca as faixas dos jogos: um clipe por vez, com play/pause,
// seek e troca de faixa mantendo a posição (o Bandle entrega um mp3 já
// mixado por estágio; ao liberar um instrumento basta trocar o arquivo).
package audio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/mp3"
)

// Rate é a taxa do dispositivo. As faixas do Bandle são 44,1 kHz; outras são
// reamostradas na decodificação.
const Rate beep.SampleRate = 44100

// maxTrackBytes limita o que Decode lê (os clipes do Bandle têm ~200 KB).
const maxTrackBytes = 32 << 20

// Track é um clipe decodificado inteiro em memória, na taxa do dispositivo.
// Manter tudo em memória torna seek e troca de faixa instantâneos.
type Track struct {
	buf *beep.Buffer
	end int // amostras visíveis; 0 = o buffer inteiro (ver Head)
}

// Decode lê um mp3 inteiro de r e o decodifica.
func Decode(r io.Reader) (*Track, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxTrackBytes))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("áudio vazio")
	}
	stream, format, err := mp3.Decode(readSeekCloser{bytes.NewReader(data)})
	if err != nil {
		return nil, fmt.Errorf("decodificar mp3: %w", err)
	}
	defer stream.Close()

	var src beep.Streamer = stream
	if format.SampleRate != Rate {
		src = beep.Resample(4, format.SampleRate, Rate, stream)
	}
	buf := beep.NewBuffer(beep.Format{SampleRate: Rate, NumChannels: 2, Precision: 2})
	buf.Append(src)
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("ler mp3: %w", err)
	}
	if buf.Len() == 0 {
		return nil, errors.New("mp3 sem amostras")
	}
	return &Track{buf: buf}, nil
}

// Duration é a duração total do clipe.
func (t *Track) Duration() time.Duration { return Rate.D(t.Samples()) }

// Samples é o tamanho em amostras.
func (t *Track) Samples() int {
	if t.end > 0 {
		return t.end
	}
	return t.buf.Len()
}

// Head devolve só os primeiros d do clipe, sem copiar o áudio. O player para
// sozinho no último sample, então o corte é exato: não depende do tick da
// tela nem do buffer do dispositivo (que, com Pause, ainda tocaria o que já
// estava enfileirado). d além da duração devolve o clipe inteiro.
func (t *Track) Head(d time.Duration) *Track {
	n := Rate.N(d)
	if n <= 0 || n >= t.Samples() {
		return t
	}
	return &Track{buf: t.buf, end: n}
}

// streamer devolve um leitor posicionável sobre o clipe inteiro.
func (t *Track) streamer() beep.StreamSeeker { return t.buf.Streamer(0, t.Samples()) }

// readSeekCloser dá Close a um bytes.Reader sem esconder o Seek, que o
// decoder precisa para saber o tamanho.
type readSeekCloser struct{ *bytes.Reader }

func (readSeekCloser) Close() error { return nil }
