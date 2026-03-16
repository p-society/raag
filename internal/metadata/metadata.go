package metadata

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	"github.com/dhowden/tag"
)

type Song struct {
	Title  string
	Artist string
	Album  string
	Path   string
	Hash   string
	Size   int64
}

func ExtractMetadata(filePath string) (Song, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return Song{}, fmt.Errorf("failed to open song file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return Song{}, fmt.Errorf("failed to stat song file: %w", err)
	}

	metadata, err := tag.ReadFrom(file)
	if err != nil {
		return Song{}, fmt.Errorf("failed to read metadata: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return Song{}, fmt.Errorf("failed to seek file for hashing: %w", err)
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return Song{}, fmt.Errorf("failed to compute hash: %w", err)
	}

	hash := fmt.Sprintf("%x", hasher.Sum(nil))
	return Song{
		Title:  metadata.Title(),
		Artist: metadata.Artist(),
		Album:  metadata.Album(),
		Path:   filePath,
		Hash:   hash,
		Size:   stat.Size(),
	}, nil
}
