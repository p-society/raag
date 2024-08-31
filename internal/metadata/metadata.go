package metadata

import (
	"fmt"
	"os"

	"github.com/dhowden/tag"
)

type Song struct {
	Title  string
	Artist string
	Album  string
	Path   string
}

func ExtractMetadata(filePath string) (Song, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return Song{}, fmt.Errorf("failed to open song file: %w", err)
	}
	defer file.Close()

	metadata, err := tag.ReadFrom(file)
	if err != nil {
		return Song{}, fmt.Errorf("failed to read metadata: %w", err)
	}

	return Song{
		Title:  metadata.Title(),
		Artist: metadata.Artist(),
		Album:  metadata.Album(),
		Path:   filePath,
	}, nil
}

func FormatMetadata(song Song) string {
	return fmt.Sprintf("%s|%s|%s", song.Title, song.Artist, song.Album)
}
