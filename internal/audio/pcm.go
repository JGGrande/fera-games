package audio

import (
	"encoding/binary"
	"errors"
	"io"
)

// maxPCMBytes limita o que DecodePCM lê: 60 s de s16le estéreo a 44,1 kHz
// são ~10 MB; o teto folgado só protege contra uma fonte descontrolada.
const maxPCMBytes = 32 << 20

// pcmFrame é o tamanho de uma amostra estéreo s16le (2 canais × 2 bytes).
const pcmFrame = 4

// DecodePCM lê PCM cru (s16le, estéreo, na taxa do dispositivo) de r e o
// transforma em Track. É o formato que o ffmpeg entrega com
// `-f s16le -ar 44100 -ac 2`, sem container nem decodificação.
func DecodePCM(r io.Reader) (*Track, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxPCMBytes))
	if err != nil {
		return nil, err
	}
	n := len(data) / pcmFrame
	if n == 0 {
		return nil, errors.New("áudio vazio")
	}
	samples := make([][2]float64, n)
	for i := range samples {
		off := i * pcmFrame
		samples[i] = [2]float64{
			float64(int16(binary.LittleEndian.Uint16(data[off:]))) / 32768,
			float64(int16(binary.LittleEndian.Uint16(data[off+2:]))) / 32768,
		}
	}
	return trackFromSamples(samples, Rate)
}
