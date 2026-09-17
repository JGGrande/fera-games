package audio

import (
	"sync"
	"time"
)

// silentPlayer é o Player de --mute e de máquinas sem dispositivo: não emite
// som, mas avança a posição pelo relógio para a interface se comportar igual.
type silentPlayer struct {
	mu       sync.Mutex
	now      func() time.Time
	length   time.Duration
	base     time.Duration // posição quando startedAt foi marcado
	started  time.Time
	playing  bool
	loop     bool
	hasTrack bool
}

// NewSilent cria um player mudo.
func NewSilent() Player { return &silentPlayer{now: time.Now} }

func (p *silentPlayer) pos() time.Duration {
	if !p.playing {
		return p.base
	}
	pos := p.base + p.now().Sub(p.started)
	if pos < p.length || p.length == 0 {
		return pos
	}
	if p.loop {
		return pos % p.length
	}
	return p.length
}

// settle grava a posição corrente em base e, se a faixa acabou sem loop, pausa.
func (p *silentPlayer) settle() {
	pos := p.pos()
	if p.playing && !p.loop && pos >= p.length {
		p.playing = false
	}
	p.base, p.started = pos, p.now()
}

func (p *silentPlayer) Load(t *Track) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.length, p.base, p.playing, p.hasTrack = t.Duration(), 0, false, true
}

func (p *silentPlayer) Swap(t *Track) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.settle()
	p.length, p.hasTrack = t.Duration(), true
	p.base = min(p.base, p.length)
}

func (p *silentPlayer) Restart(t *Track) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.length, p.base, p.hasTrack = t.Duration(), 0, true
	p.playing, p.started = true, p.now()
}

func (p *silentPlayer) Play() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.hasTrack {
		return
	}
	p.settle()
	if p.base >= p.length {
		p.base = 0
	}
	p.playing, p.started = true, p.now()
}

func (p *silentPlayer) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.settle()
	p.playing = false
}

func (p *silentPlayer) Toggle() {
	if p.Playing() {
		p.Pause()
	} else {
		p.Play()
	}
}

func (p *silentPlayer) Seek(delta time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.settle()
	p.base = max(0, min(p.base+delta, p.length))
}

func (p *silentPlayer) SeekTo(pos time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.settle()
	p.base = max(0, min(pos, p.length))
}

func (p *silentPlayer) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.settle()
	return p.base
}

func (p *silentPlayer) Duration() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.length
}

func (p *silentPlayer) Playing() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.settle()
	return p.playing
}

func (p *silentPlayer) SetLoop(on bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.settle()
	p.loop = on
}

func (p *silentPlayer) Silent() bool { return true }
func (p *silentPlayer) Close() error { p.Pause(); return nil }
