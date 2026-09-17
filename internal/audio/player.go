package audio

import (
	"fmt"
	"sync"
	"time"

	"github.com/gopxl/beep/v2/speaker"
)

// Player é o que a TUI usa. Todos os métodos são seguros para chamar de
// qualquer goroutine.
type Player interface {
	// Load define a faixa, volta ao início e pausa.
	Load(t *Track)
	// Swap troca a faixa mantendo posição e estado de reprodução.
	Swap(t *Track)
	// Restart troca a faixa, volta ao início e toca (novo estágio do Bandle).
	Restart(t *Track)
	Play()
	Pause()
	Toggle()
	// Seek move a posição relativamente (negativo volta), com clamp.
	Seek(delta time.Duration)
	SeekTo(pos time.Duration)
	Position() time.Duration
	Duration() time.Duration
	Playing() bool
	SetLoop(on bool)
	// Silent é verdadeiro quando não há som de fato (--mute ou sem dispositivo).
	Silent() bool
	Close() error
}

var (
	initOnce sync.Once
	initErr  error
)

// Open abre o dispositivo de áudio e devolve um Player real. Se não houver
// dispositivo, devolve erro; o chamador decide cair para NewSilent.
func Open() (Player, error) {
	initOnce.Do(func() {
		// 100 ms de buffer: latência baixa o bastante para play/pause parecerem imediatos.
		initErr = speaker.Init(Rate, Rate.N(100*time.Millisecond))
	})
	if initErr != nil {
		return nil, fmt.Errorf("abrir dispositivo de áudio: %w", initErr)
	}
	p := &speakerPlayer{deck: newDeck()}
	speaker.Play(p.deck)
	return p, nil
}

// speakerPlayer liga o deck ao speaker do beep; cada operação entra no lock
// do speaker, que é o que serializa o acesso ao deck.
type speakerPlayer struct {
	deck *deck
}

func (p *speakerPlayer) lock(f func()) {
	speaker.Lock()
	defer speaker.Unlock()
	f()
}

func (p *speakerPlayer) Load(t *Track)    { p.lock(func() { p.deck.load(t) }) }
func (p *speakerPlayer) Swap(t *Track)    { p.lock(func() { p.deck.swap(t) }) }
func (p *speakerPlayer) Restart(t *Track) { p.lock(func() { p.deck.restart(t) }) }
func (p *speakerPlayer) Play() {
	p.lock(func() {
		if p.deck.cur == nil {
			return
		}
		if p.deck.atEnd() {
			p.deck.seekTo(0)
		}
		p.deck.playing = true
	})
}
func (p *speakerPlayer) Pause() { p.lock(func() { p.deck.playing = false }) }
func (p *speakerPlayer) Toggle() {
	if p.Playing() {
		p.Pause()
	} else {
		p.Play()
	}
}
func (p *speakerPlayer) Seek(delta time.Duration) {
	p.lock(func() { p.deck.seekTo(p.deck.position() + Rate.N(delta)) })
}
func (p *speakerPlayer) SeekTo(pos time.Duration) { p.lock(func() { p.deck.seekTo(Rate.N(pos)) }) }
func (p *speakerPlayer) Position() (d time.Duration) {
	p.lock(func() { d = Rate.D(p.deck.position()) })
	return
}
func (p *speakerPlayer) Duration() (d time.Duration) {
	p.lock(func() { d = Rate.D(p.deck.length) })
	return
}
func (p *speakerPlayer) Playing() (b bool) {
	p.lock(func() { b = p.deck.playing })
	return
}
func (p *speakerPlayer) SetLoop(on bool) { p.lock(func() { p.deck.loop = on }) }
func (p *speakerPlayer) Silent() bool    { return false }

// Close para a reprodução. O speaker fica aberto: é global e barato.
func (p *speakerPlayer) Close() error {
	p.lock(func() { p.deck.playing = false; p.deck.cur, p.deck.old = nil, nil })
	return nil
}
