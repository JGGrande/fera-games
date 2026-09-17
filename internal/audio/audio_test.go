package audio

import (
	"bytes"
	"math"
	"testing"
	"time"

	"github.com/gopxl/beep/v2"

	"fera-games/internal/audio/audiotest"
)

// constTrack cria uma faixa de n amostras com valor constante v nos dois canais.
func constTrack(n int, v float64) *Track {
	buf := beep.NewBuffer(beep.Format{SampleRate: Rate, NumChannels: 2, Precision: 2})
	buf.Append(&constStreamer{n: n, v: v})
	return &Track{buf: buf}
}

type constStreamer struct {
	n int
	v float64
}

func (s *constStreamer) Stream(out [][2]float64) (int, bool) {
	if s.n == 0 {
		return 0, false
	}
	n := min(len(out), s.n)
	for i := range out[:n] {
		out[i] = [2]float64{s.v, s.v}
	}
	s.n -= n
	return n, true
}
func (s *constStreamer) Err() error { return nil }

// pull lê n amostras do deck.
func pull(d *deck, n int) [][2]float64 {
	out := make([][2]float64, n)
	d.Stream(out)
	return out
}

func TestDeckPausedIsSilence(t *testing.T) {
	d := newDeck()
	for _, v := range pull(d, 16) {
		if v[0] != 0 {
			t.Fatal("deck vazio deveria gerar silêncio")
		}
	}
	d.load(constTrack(1000, 0.5))
	if d.playing {
		t.Fatal("load deveria deixar pausado")
	}
	if v := pull(d, 16); v[0][0] != 0 || d.position() != 0 {
		t.Fatalf("pausado gerou som ou avançou: %v pos=%d", v[0], d.position())
	}
}

func TestDeckPlaysAndStopsAtEnd(t *testing.T) {
	d := newDeck()
	d.load(constTrack(100, 0.5))
	d.playing = true
	out := pull(d, 150)
	if math.Abs(out[0][0]-0.5) > 1e-3 || math.Abs(out[99][1]-0.5) > 1e-3 {
		t.Fatalf("amostras erradas: %v %v", out[0], out[99])
	}
	if out[100][0] != 0 || out[149][0] != 0 {
		t.Fatal("depois do fim deveria vir silêncio")
	}
	if d.playing || !d.atEnd() || d.position() != 100 {
		t.Fatalf("fim: playing=%v atEnd=%v pos=%d", d.playing, d.atEnd(), d.position())
	}
}

func TestDeckLoop(t *testing.T) {
	d := newDeck()
	d.load(constTrack(100, 0.5))
	d.playing, d.loop = true, true
	out := pull(d, 250)
	for i, v := range out {
		if math.Abs(v[0]-0.5) > 1e-3 {
			t.Fatalf("amostra %d em loop = %v", i, v)
		}
	}
	if !d.playing || d.position() != 50 {
		t.Fatalf("loop: playing=%v pos=%d", d.playing, d.position())
	}
}

func TestDeckSwapKeepsPositionAndCrossfades(t *testing.T) {
	d := newDeck()
	d.fadeLen = 10 // fade curto para o teste
	d.load(constTrack(1000, 0.2))
	d.playing = true
	pull(d, 300)
	if d.position() != 300 {
		t.Fatalf("pos = %d", d.position())
	}

	d.swap(constTrack(1000, 1.0))
	if d.position() != 300 || !d.playing {
		t.Fatalf("swap mudou posição/estado: pos=%d playing=%v", d.position(), d.playing)
	}
	out := pull(d, 20)
	// Durante o fade a saída sobe de 0.2 para 1.0 sem saltar; depois fica em 1.0.
	prev := 0.0
	for i := 0; i < 10; i++ {
		v := out[i][0]
		if v < prev-1e-3 || v < 0.2-1e-3 || v > 1.0+1e-3 { // buffer de 16 bits quantiza
			t.Fatalf("fade não monotônico em %d: %v", i, out[:10])
		}
		if i > 0 && v-prev > 0.2 {
			t.Fatalf("salto grande no fade em %d: %v → %v", i, prev, v)
		}
		prev = v
	}
	for i := 10; i < 20; i++ {
		if math.Abs(out[i][0]-1.0) > 1e-3 {
			t.Fatalf("após o fade esperava 1.0, veio %v em %d", out[i][0], i)
		}
	}
	if d.position() != 320 || d.old != nil || d.fade != 0 {
		t.Fatalf("pós-swap: pos=%d old=%v fade=%d", d.position(), d.old != nil, d.fade)
	}
}

func TestDeckRestartFromZeroWithFade(t *testing.T) {
	d := newDeck()
	d.fadeLen = 10
	d.load(constTrack(1000, 0.2))
	d.playing = true
	pull(d, 500)

	d.restart(constTrack(1000, 1.0))
	if d.position() != 0 || !d.playing || d.fade != 10 {
		t.Fatalf("restart: pos=%d playing=%v fade=%d", d.position(), d.playing, d.fade)
	}
	out := pull(d, 20)
	if out[0][0] > 0.3 || math.Abs(out[19][0]-1.0) > 1e-3 {
		t.Fatalf("fade do restart: início %v, fim %v", out[0][0], out[19][0])
	}
	if d.position() != 20 {
		t.Fatalf("pos após restart = %d", d.position())
	}

	// Pausado: restart também começa a tocar, sem fade.
	d.playing = false
	d.restart(constTrack(100, 0.5))
	if !d.playing || d.fade != 0 || d.position() != 0 {
		t.Fatalf("restart pausado: playing=%v fade=%d pos=%d", d.playing, d.fade, d.position())
	}
}

func TestDeckSwapWhilePausedHasNoFade(t *testing.T) {
	d := newDeck()
	d.load(constTrack(1000, 0.2))
	d.seekTo(400)
	d.swap(constTrack(1000, 1.0))
	if d.fade != 0 || d.position() != 400 || d.playing {
		t.Fatalf("swap pausado: fade=%d pos=%d playing=%v", d.fade, d.position(), d.playing)
	}
	d.swap(constTrack(200, 1.0)) // faixa mais curta que a posição atual
	if d.position() != 200 {
		t.Fatalf("posição deveria ser limitada ao tamanho da nova faixa, veio %d", d.position())
	}
}

func TestDeckSeekClamps(t *testing.T) {
	d := newDeck()
	d.load(constTrack(100, 0.5))
	d.seekTo(-5)
	if d.position() != 0 {
		t.Fatalf("seek negativo: %d", d.position())
	}
	d.seekTo(500)
	if d.position() != 100 {
		t.Fatalf("seek além do fim: %d", d.position())
	}
}

func TestDecodeSyntheticMP3(t *testing.T) {
	tr, err := Decode(bytes.NewReader(audiotest.SilentMP3(20)))
	if err != nil {
		t.Fatal(err)
	}
	if tr.Samples() != 20*1152 || tr.Duration().Round(time.Millisecond) != 522*time.Millisecond {
		t.Fatalf("amostras=%d dur=%s", tr.Samples(), tr.Duration())
	}
	if _, err := Decode(bytes.NewReader([]byte("nao e mp3"))); err == nil {
		t.Fatal("lixo deveria falhar")
	}
	if _, err := Decode(bytes.NewReader(nil)); err == nil {
		t.Fatal("vazio deveria falhar")
	}
}

func TestSilentPlayerFollowsClock(t *testing.T) {
	now := time.Unix(0, 0)
	p := &silentPlayer{now: func() time.Time { return now }}
	track := constTrack(int(Rate)*10, 0) // 10 s

	p.Play() // sem faixa: ignora
	if p.Playing() {
		t.Fatal("sem faixa não deveria tocar")
	}
	p.Load(track)
	if p.Duration() != 10*time.Second || p.Position() != 0 || p.Playing() {
		t.Fatalf("load: dur=%s pos=%s playing=%v", p.Duration(), p.Position(), p.Playing())
	}
	p.Play()
	now = now.Add(3 * time.Second)
	if p.Position() != 3*time.Second {
		t.Fatalf("pos = %s", p.Position())
	}
	p.Seek(-1 * time.Second)
	if p.Position() != 2*time.Second {
		t.Fatalf("seek: %s", p.Position())
	}
	p.Swap(constTrack(int(Rate)*10, 0))
	now = now.Add(time.Second)
	if p.Position() != 3*time.Second || !p.Playing() {
		t.Fatalf("swap perdeu posição: %s %v", p.Position(), p.Playing())
	}
	p.Pause()
	now = now.Add(5 * time.Second)
	if p.Position() != 3*time.Second || p.Playing() {
		t.Fatalf("pausado avançou: %s", p.Position())
	}
	p.Play()
	now = now.Add(20 * time.Second)
	if p.Position() != 10*time.Second || p.Playing() {
		t.Fatalf("fim: pos=%s playing=%v", p.Position(), p.Playing())
	}
	p.Play() // recomeça do zero
	if p.Position() != 0 || !p.Playing() {
		t.Fatalf("replay: pos=%s playing=%v", p.Position(), p.Playing())
	}
	p.SetLoop(true)
	now = now.Add(23 * time.Second)
	if p.Position() != 3*time.Second || !p.Playing() {
		t.Fatalf("loop: pos=%s playing=%v", p.Position(), p.Playing())
	}
	p.Pause()
	p.Restart(constTrack(int(Rate)*5, 0))
	now = now.Add(2 * time.Second)
	if p.Position() != 2*time.Second || !p.Playing() || p.Duration() != 5*time.Second {
		t.Fatalf("restart mudo: pos=%s playing=%v dur=%s", p.Position(), p.Playing(), p.Duration())
	}
	if !p.Silent() {
		t.Fatal("Silent() deveria ser true")
	}
}
