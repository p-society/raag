package player

import (
	"fmt"
	"os"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"github.com/p-society/raag/internal/library"
)

type Player struct {
	ctrl   *beep.Ctrl
	format beep.Format
}

func NewPlayer() *Player {
	return &Player{}
}

func (p *Player) Play(song library.Song) error {
	f, err := os.Open(song.Path)
	if err != nil {
		return fmt.Errorf("error opening audio file: %w", err)
	}

	streamer, format, err := mp3.Decode(f)
	if err != nil {
		return fmt.Errorf("error decoding audio file: %w", err)
	}

	speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))
	p.ctrl = &beep.Ctrl{Streamer: beep.Loop(-1, streamer)}
	speaker.Play(p.ctrl)
	p.format = format

	fmt.Printf("Now playing: %s - %s\n", song.Title, song.Artist)
	return nil
}

func (p *Player) Pause() {
	if p.ctrl != nil {
		speaker.Lock()
		p.ctrl.Paused = true
		speaker.Unlock()
		fmt.Println("Playback paused")
	}
}

func (p *Player) Resume() {
	if p.ctrl != nil {
		speaker.Lock()
		p.ctrl.Paused = false
		speaker.Unlock()
		fmt.Println("Playback resumed")
	}
}

func (p *Player) Stop() {
	if p.ctrl != nil {
		speaker.Clear()
		fmt.Println("Playback stopped")
	}
}
