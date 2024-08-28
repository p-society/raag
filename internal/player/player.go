package player

import (
	"fmt"
	"os"
	"time"

	tag "github.com/dhowden/tag"
	beep "github.com/faiface/beep"
	mp3 "github.com/faiface/beep/mp3"
	speaker "github.com/faiface/beep/speaker"
)

func PlayMusic(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	mdata, err := tag.ReadFrom(file)
	if err != nil {
		return fmt.Errorf("error reading tag: %w", err)
	}

	fmt.Printf("Now playing %s by %s\n", mdata.Title(), mdata.Artist())
	_, err = file.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("error seeking file: %w", err)
	}

	stream, format, err := mp3.Decode(file)
	if err != nil {
		return fmt.Errorf("error decoding mp3 file: %w", err)
	}
	defer stream.Close()

	err = speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))
	if err != nil {
		return fmt.Errorf("error initializing speaker: %w", err)
	}

	done := make(chan bool)
	speaker.Play(beep.Seq(stream, beep.Callback(func() {
		done <- true
	})))

	<-done
	return nil
}
