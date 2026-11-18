package audio

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// pcmBytes gera n amostras estéreo s16le com valores l e r.
func pcmBytes(n int, l, r int16) []byte {
	b := make([]byte, 0, n*pcmFrame)
	for i := 0; i < n; i++ {
		b = binary.LittleEndian.AppendUint16(b, uint16(l))
		b = binary.LittleEndian.AppendUint16(b, uint16(r))
	}
	return b
}

func TestDecodePCM(t *testing.T) {
	tr, err := DecodePCM(bytes.NewReader(pcmBytes(Rate.N(time.Second), 16384, -16384)))
	if err != nil {
		t.Fatal(err)
	}
	if tr.Samples() != Rate.N(time.Second) {
		t.Fatalf("samples = %d, quero %d", tr.Samples(), Rate.N(time.Second))
	}
	if d := tr.Duration(); d != time.Second {
		t.Fatalf("duração = %v", d)
	}
	out := make([][2]float64, 4)
	if n, _ := tr.streamer().Stream(out); n != 4 {
		t.Fatalf("n = %d", n)
	}
	if out[0][0] != 0.5 || out[0][1] != -0.5 {
		t.Fatalf("amostra = %v, quero [0.5 -0.5]", out[0])
	}
}

func TestDecodePCMDescartaBytesSoltos(t *testing.T) {
	data := append(pcmBytes(100, 1, 1), 0x7F, 0x7F, 0x7F) // 3 bytes de lixo no fim
	tr, err := DecodePCM(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if tr.Samples() != 100 {
		t.Fatalf("samples = %d, quero 100", tr.Samples())
	}
}

func TestDecodePCMVazio(t *testing.T) {
	for _, in := range [][]byte{nil, {1, 2, 3}} {
		if _, err := DecodePCM(bytes.NewReader(in)); err == nil {
			t.Fatalf("esperava erro para %d bytes", len(in))
		}
	}
}

func TestHeadCortaSemCopiarEODeckParaNoCorte(t *testing.T) {
	full, err := DecodePCM(bytes.NewReader(pcmBytes(Rate.N(2*time.Second), 16384, 16384)))
	if err != nil {
		t.Fatal(err)
	}
	head := full.Head(130 * time.Millisecond)
	if head.Samples() != Rate.N(130*time.Millisecond) || head.Duration() >= 131*time.Millisecond {
		t.Fatalf("Head: %d amostras, %v", head.Samples(), head.Duration())
	}
	if full.Samples() != Rate.N(2*time.Second) {
		t.Fatal("Head não pode mudar a faixa original")
	}
	if full.Head(time.Hour) != full || full.Head(0) != full {
		t.Fatal("corte fora da faixa devolve a faixa inteira")
	}
	if h := head.Head(time.Second); h.Samples() != head.Samples() {
		t.Fatal("Head de um Head maior não pode crescer além do corte")
	}

	// O deck toca exatamente as amostras do corte e depois só silêncio, mesmo
	// pedindo bem mais (como faz o speaker com o buffer de 100 ms).
	d := newDeck()
	d.restart(head)
	out := pull(d, Rate.N(400*time.Millisecond))
	sound := 0
	for _, v := range out {
		if v[0] != 0 {
			sound++
		}
	}
	if sound != head.Samples() {
		t.Fatalf("tocou %d amostras com som, quero exatamente %d", sound, head.Samples())
	}
	if d.playing {
		t.Fatal("o deck deveria parar sozinho no fim do corte")
	}
}
