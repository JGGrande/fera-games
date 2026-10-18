package audio

import (
	"bytes"
	"testing"
	"time"

	"fera-games/internal/games/songless/fixture"
)

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

func TestDecodeAACReal(t *testing.T) {
	tr, err := DecodeAAC(bytesReader(fixture.ShortAAC()))
	if err != nil {
		t.Fatal(err)
	}
	// 30 frames de 1024 amostras a 48 kHz = 640 ms; reamostrado para 44.1 kHz
	// deve ter duração parecida (tolerância para diferenças de decoder).
	dur := tr.Duration()
	if dur < 600*time.Millisecond || dur > 680*time.Millisecond {
		t.Fatalf("duração = %s, esperava ~640 ms", dur)
	}
	if tr.Samples() == 0 {
		t.Fatal("sem amostras")
	}
	if _, err := DecodeAAC(bytesReader(nil)); err == nil {
		t.Fatal("vazio deveria falhar")
	}
	if _, err := DecodeAAC(bytesReader([]byte("nao e aac"))); err == nil {
		t.Fatal("lixo deveria falhar")
	}
}
