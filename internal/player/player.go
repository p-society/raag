package player

import (
	"fmt"
	"os"

	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"github.com/p-society/raag/internal/metadata"
)

type Player struct {
	ctrl     *beep.Ctrl
	format   beep.Format
	streamer beep.StreamSeeker
}

func NewPlayer() (*Player, error) {
	err := speaker.Init(44100, 44100/10)
	if err != nil {
		return nil, fmt.Errorf("error initializing speaker: %w", err)
	}
	return &Player{}, nil
}

func (p *Player) Play(song metadata.Song) error {
	if p.streamer != nil {
		speaker.Clear()
	}

	f, err := os.Open(song.Path)
	if err != nil {
		return fmt.Errorf("error opening audio file: %w", err)
	}

	streamer, format, err := mp3.Decode(f)
	if err != nil {
		f.Close()
		return fmt.Errorf("error decoding audio file: %w", err)
	}

	p.streamer = streamer
	p.format = format
	p.ctrl = &beep.Ctrl{Streamer: beep.Loop(-1, streamer)}

	speaker.Play(p.ctrl)

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
