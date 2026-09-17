// Package audiotest fabrica áudio para testes sem depender de arquivos reais.
package audiotest

import "bytes"

// SilentMP3 gera um mp3 válido de frames zerados (MPEG-1 Layer III, 128 kbps,
// 44,1 kHz, estéreo): cada frame decodifica para 1152 amostras de silêncio.
func SilentMP3(frames int) []byte {
	frame := make([]byte, 417)
	frame[0], frame[1], frame[2], frame[3] = 0xFF, 0xFB, 0x90, 0x00
	var buf bytes.Buffer
	for i := 0; i < frames; i++ {
		buf.Write(frame)
	}
	return buf.Bytes()
}
