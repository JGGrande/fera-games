package audio

import (
	"time"

	"github.com/gopxl/beep/v2"
)

// fadeDuration é o crossfade aplicado na troca de faixa e nos saltos, para
// não estalar quando a forma de onda muda de repente.
const fadeDuration = 20 * time.Millisecond

// deck é a lógica do player como um beep.Streamer: independe do dispositivo
// e por isso é testável. Quem o usa deve serializar o acesso (o speaker faz
// isso com Lock/Unlock).
type deck struct {
	cur     beep.StreamSeeker // faixa atual; nil = nada carregado
	old     beep.StreamSeeker // faixa anterior, enquanto o crossfade dura
	fade    int               // amostras restantes do crossfade
	fadeLen int
	playing bool
	loop    bool
	length  int
}

func newDeck() *deck { return &deck{fadeLen: Rate.N(fadeDuration)} }

// load troca a faixa e volta ao início, pausado.
func (d *deck) load(t *Track) {
	d.cur, d.old, d.fade = t.streamer(), nil, 0
	d.length = t.Samples()
	d.playing = false
}

// swap troca a faixa mantendo posição e estado, com crossfade da antiga para a nova.
func (d *deck) swap(t *Track) {
	if d.cur == nil {
		d.load(t)
		return
	}
	d.replace(t, min(d.cur.Position(), t.Samples()))
}

// restart troca a faixa, volta ao início e toca; a antiga sai em fade.
func (d *deck) restart(t *Track) {
	if d.cur == nil {
		d.load(t)
	} else {
		d.replace(t, 0)
	}
	d.playing = true
}

// replace instala t na posição pos. Se algo estava tocando, a faixa antiga
// segue por fadeLen amostras misturada com a nova para não estalar.
func (d *deck) replace(t *Track, pos int) {
	next := t.streamer()
	_ = next.Seek(pos)
	if d.playing && d.cur.Position() < d.cur.Len() {
		d.old, d.fade = d.cur, d.fadeLen
	} else {
		d.old, d.fade = nil, 0
	}
	d.cur = next
	d.length = t.Samples()
}

func (d *deck) position() int {
	if d.cur == nil {
		return 0
	}
	return d.cur.Position()
}

// seekTo posiciona (com clamp) e descarta qualquer crossfade em andamento.
func (d *deck) seekTo(p int) {
	if d.cur == nil {
		return
	}
	p = max(0, min(p, d.length))
	_ = d.cur.Seek(p)
	d.old, d.fade = nil, 0
}

// atEnd é verdadeiro quando a faixa terminou e não está em loop.
func (d *deck) atEnd() bool { return d.cur != nil && d.cur.Position() >= d.length }

// Stream implementa beep.Streamer. Pausado ou vazio, gera silêncio e nunca
// "termina": o speaker fica com o deck instalado pelo processo inteiro.
func (d *deck) Stream(samples [][2]float64) (int, bool) {
	if !d.playing || d.cur == nil {
		clear(samples)
		return len(samples), true
	}
	n := 0
	for n < len(samples) {
		if d.atEnd() {
			if !d.loop {
				d.playing = false
				break
			}
			_ = d.cur.Seek(0)
			d.old, d.fade = nil, 0
		}
		want := len(samples) - n
		if d.fade > 0 {
			want = min(want, d.fade)
		}
		got, _ := d.cur.Stream(samples[n : n+want])
		if got == 0 {
			break
		}
		if d.fade > 0 {
			d.mixOld(samples[n : n+got])
		}
		n += got
	}
	clear(samples[n:])
	return len(samples), true
}

// mixOld faz o crossfade linear: a antiga vai de 1 a 0 e a nova de 0 a 1.
func (d *deck) mixOld(out [][2]float64) {
	if d.old == nil {
		d.fade = 0
		return
	}
	tmp := make([][2]float64, len(out))
	got, _ := d.old.Stream(tmp)
	for i := range out {
		// progresso do fade em [0,1): 0 = só a antiga, 1 = só a nova
		t := 1 - float64(d.fade)/float64(d.fadeLen)
		if i < got {
			out[i][0] = out[i][0]*t + tmp[i][0]*(1-t)
			out[i][1] = out[i][1]*t + tmp[i][1]*(1-t)
		}
		d.fade--
		if d.fade <= 0 {
			d.fade, d.old = 0, nil
			return
		}
	}
}

// Err implementa beep.Streamer.
func (d *deck) Err() error { return nil }
