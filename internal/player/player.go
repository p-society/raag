package player

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"
	"github.com/faiface/beep/flac"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"github.com/faiface/beep/vorbis"
	"github.com/faiface/beep/wav"
	"github.com/p-society/raag/internal/logger"
	"github.com/p-society/raag/internal/metadata"
)

type Player struct {
	ctrl         *beep.Ctrl
	format       beep.Format
	streamer     beep.StreamSeeker
	streamCloser beep.StreamSeekCloser
	file         *os.File
	Queue        []metadata.Song
	CurrentIndex int
	Volume       float64
	Position     int
	Duration     int
	VolumeCtrl   *effects.Volume
	mutex        sync.RWMutex
	playerNext   func() error
	trackEnded   chan struct{}
}

func NewPlayer() (*Player, error) {
	err := speaker.Init(44100, 44100/10)
	if err != nil {
		return nil, fmt.Errorf("error initializing speaker: %w", err)
	}
	return &Player{
		Volume:       0.5,
		CurrentIndex: -1,
		Queue:        []metadata.Song{},
	}, nil
}

func (p *Player) Play(song metadata.Song) error {
	f, err := os.Open(song.Path)
	if err != nil {
		return fmt.Errorf("error opening audio file: %w", err)
	}

	streamer, format, err := p.decodeAudio(f)
	if err != nil {
		f.Close()
		return fmt.Errorf("error decoding audio file: %w", err)
	}

	volume := p.currentVolume()
	duration := 0
	if streamer.Len() > 0 {
		duration = streamer.Len() / int(format.SampleRate)
	}

	p.mutex.Lock()
	p.stopPlaybackLocked() // Clean up old state first

	p.trackEnded = make(chan struct{}, 1)
	trackEnded := p.trackEnded

	p.file = f
	p.streamer = streamer
	p.streamCloser = streamer
	p.format = format
	p.Position = 0
	p.Duration = duration

	seq := beep.Seq(streamer, beep.Callback(func() {
		select {
		case trackEnded <- struct{}{}:
		default:
		}
	}))

	p.ctrl = &beep.Ctrl{Streamer: seq}
	p.VolumeCtrl = &effects.Volume{Streamer: p.ctrl, Base: 2, Volume: volume}
	p.mutex.Unlock()

	speaker.Play(p.VolumeCtrl)
	currentTrackEnded := trackEnded
	go func() {
		_, ok := <-currentTrackEnded
		if !ok {
			return
		}
		if p.playerNext != nil {
			if err := p.playerNext(); err != nil {
				logger.Debugf("Queue finished or error playing next: %v", err)
			}
		}
	}()

	fmt.Printf("Now playing: %s - %s\n", song.Title, song.Artist)
	return nil
}

func (p *Player) currentVolume() float64 {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.Volume
}

func (p *Player) decodeAudio(f *os.File) (beep.StreamSeekCloser, beep.Format, error) {
	ext := strings.ToLower(filepath.Ext(f.Name()))
	switch ext {
	case ".mp3":
		return mp3.Decode(f)
	case ".flac":
		return flac.Decode(f)
	case ".wav":
		return wav.Decode(f)
	case ".ogg", ".ogv":
		return vorbis.Decode(f)
	default:
		return nil, beep.Format{}, fmt.Errorf("unsupported audio format: %s", ext)
	}
}

func (p *Player) PlayQueue() error {
	song, err := p.selectSongAtIndex(0)
	if err != nil {
		return err
	}
	return p.Play(song)
}

func (p *Player) selectSongAtIndex(index int) (metadata.Song, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if len(p.Queue) == 0 {
		return metadata.Song{}, fmt.Errorf("queue is empty")
	}
	if index < 0 || index >= len(p.Queue) {
		return metadata.Song{}, fmt.Errorf("invalid index")
	}

	p.CurrentIndex = index
	return p.Queue[index], nil
}

func (p *Player) Pause() {
	p.mutex.RLock()
	ctrl := p.ctrl
	p.mutex.RUnlock()

	if ctrl != nil {
		speaker.Lock()
		ctrl.Paused = true
		speaker.Unlock()
		logger.Infof("Playback paused")
	}
}

func (p *Player) Resume() {
	p.mutex.RLock()
	ctrl := p.ctrl
	p.mutex.RUnlock()

	if ctrl != nil {
		speaker.Lock()
		ctrl.Paused = false
		speaker.Unlock()
		logger.Infof("Playback resumed")
	}
}

func (p *Player) Stop() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.stopPlaybackLocked()
	p.Queue = []metadata.Song{}
	p.CurrentIndex = -1
	p.Position = 0
	p.Duration = 0
	fmt.Println("Playback stopped and queue cleared")
}

func (p *Player) stopPlaybackLocked() {
	if p.ctrl != nil || p.streamer != nil {
		speaker.Clear()
	}
	if p.trackEnded != nil {
		close(p.trackEnded)
		p.trackEnded = nil
	}

	p.ctrl = nil
	p.streamer = nil
	p.VolumeCtrl = nil
	p.format = beep.Format{}
	if p.streamCloser != nil {
		p.streamCloser.Close()
		p.streamCloser = nil
	}
	if p.file != nil {
		p.file.Close()
		p.file = nil
	}
}

func (p *Player) AddToQueue(song metadata.Song) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.Queue = append(p.Queue, song)
}

func (p *Player) GetQueue() []metadata.Song {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	result := make([]metadata.Song, len(p.Queue))
	copy(result, p.Queue)
	return result
}

func (p *Player) Next() error {
	p.mutex.Lock()
	if len(p.Queue) == 0 {
		p.mutex.Unlock()
		return fmt.Errorf("queue is empty")
	}
	if p.CurrentIndex >= len(p.Queue)-1 {
		p.mutex.Unlock()
		return fmt.Errorf("end of queue")
	}

	p.CurrentIndex++
	song := p.Queue[p.CurrentIndex]
	p.mutex.Unlock()

	return p.Play(song)
}

func (p *Player) Previous() error {
	p.mutex.Lock()
	if len(p.Queue) == 0 {
		p.mutex.Unlock()
		return fmt.Errorf("queue is empty")
	}
	if p.CurrentIndex <= 0 {
		p.mutex.Unlock()
		return fmt.Errorf("beginning of queue")
	}

	p.CurrentIndex--
	song := p.Queue[p.CurrentIndex]
	p.mutex.Unlock()

	return p.Play(song)
}

func (p *Player) GetCurrentSong() *metadata.Song {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	if p.CurrentIndex < 0 || p.CurrentIndex >= len(p.Queue) {
		return nil
	}

	song := p.Queue[p.CurrentIndex]
	return &song
}

func (p *Player) SetVolume(level float64) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if level < 0 || level > 100 {
		return fmt.Errorf("volume must be between 0 and 100")
	}

	p.Volume = (level/100 - 1) * 10
	if p.VolumeCtrl != nil {
		speaker.Lock()
		p.VolumeCtrl.Volume = p.Volume
		speaker.Unlock()
	}
	fmt.Printf("Volume set to %.0f%%\n", level)
	return nil
}

func (p *Player) GetVolume() float64 {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return (p.Volume/10 + 1) * 100
}

func (p *Player) adjustVolume(delta float64) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	newVol := p.Volume + (delta / 10)
	if newVol > 10 {
		newVol = 10
	} else if newVol < -10 {
		newVol = -10
	}

	p.Volume = newVol
	if p.VolumeCtrl != nil {
		speaker.Lock()
		p.VolumeCtrl.Volume = p.Volume
		speaker.Unlock()
	}
	fmt.Printf("Volume: %.0f%%\n", (newVol/10+1)*100)
}

func (p *Player) VolumeUp(amount float64) {
	p.adjustVolume(amount)
}

func (p *Player) VolumeDown(amount float64) {
	p.adjustVolume(-amount)
}

func (p *Player) Seek(seconds int) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.streamer == nil {
		return fmt.Errorf("no song playing")
	}

	pos := p.format.SampleRate.N(time.Duration(seconds) * time.Second)
	if p.streamer.Len() > 0 && pos > p.streamer.Len() {
		pos = p.streamer.Len()
	}

	speaker.Lock()
	err := p.streamer.Seek(pos)
	speaker.Unlock()

	if err != nil {
		return fmt.Errorf("seek error: %w", err)
	}

	p.Position = pos / int(p.format.SampleRate)
	fmt.Printf("Seeked to %d seconds\n", seconds)
	return nil
}

func (p *Player) GetPosition() int {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if p.streamer != nil && p.format.SampleRate > 0 {
		speaker.Lock()
		pos := p.streamer.Position()
		speaker.Unlock()
		return pos / int(p.format.SampleRate)
	}
	return p.Position
}

func (p *Player) GetDuration() int {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.Duration
}

func (p *Player) IsPlaying() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.ctrl != nil && !p.ctrl.Paused
}

func (p *Player) IsPaused() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.ctrl != nil && p.ctrl.Paused
}

func (p *Player) SetNextCallback(fn func() error) {
	p.playerNext = fn
}
