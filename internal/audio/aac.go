package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/gopxl/beep/v2"
	aacpcm "github.com/tphakala/go-aac/pcm"
)

// maxAACBytes limita o que DecodeAAC lê (os .aac do Songless têm ~700 KB).
const maxAACBytes = 32 << 20

// DecodeAAC lê um ADTS AAC-LC inteiro de r (ex.: o .aac do Songless, 48 kHz
// estéreo) e o decodifica para a taxa do dispositivo, como Decode faz com mp3.
func DecodeAAC(r io.Reader) (*Track, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxAACBytes))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("áudio vazio")
	}
	d, err := aacpcm.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decodificar aac: %w", err)
	}
	info := d.Info()
	if info.Channels != 1 && info.Channels != 2 {
		return nil, fmt.Errorf("aac com %d canais, quero 1 ou 2", info.Channels)
	}
	pcm, err := io.ReadAll(d)
	if err != nil {
		return nil, fmt.Errorf("ler aac: %w", err)
	}
	if len(pcm) == 0 {
		return nil, errors.New("aac sem amostras")
	}
	n := len(pcm) / 2 // amostras S16 (intercaladas)
	samples := make([][2]float64, 0, n/2+1)
	for i := 0; i+1 < len(pcm); i += 2 {
		v := float64(int16(binary.LittleEndian.Uint16(pcm[i:]))) / 32768
		if info.Channels == 2 {
			// Junta L e R em pares.
			if (i/2)%2 == 0 {
				samples = append(samples, [2]float64{v, 0})
			} else {
				samples[len(samples)-1][1] = v
			}
		} else {
			samples = append(samples, [2]float64{v, v})
		}
	}
	if len(samples) == 0 {
		return nil, errors.New("aac sem amostras")
	}
	srcRate := beep.SampleRate(info.SampleRate)
	var src beep.Streamer = &sliceStreamer{samples: samples}
	if srcRate != Rate {
		src = beep.Resample(4, srcRate, Rate, src)
	}
	buf := beep.NewBuffer(beep.Format{SampleRate: Rate, NumChannels: 2, Precision: 2})
	buf.Append(src)
	if buf.Len() == 0 {
		return nil, errors.New("aac sem amostras")
	}
	return &Track{buf: buf}, nil
}

// sliceStreamer é um beep.Streamer sobre amostras já decodificadas.
type sliceStreamer struct {
	samples [][2]float64
	pos     int
}

func (s *sliceStreamer) Stream(out [][2]float64) (int, bool) {
	if s.pos >= len(s.samples) {
		return 0, false
	}
	n := copy(out, s.samples[s.pos:])
	s.pos += n
	return n, true
}

func (s *sliceStreamer) Err() error { return nil }
