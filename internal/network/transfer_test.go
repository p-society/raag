package network

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/p-society/raag/internal/constants"
	"github.com/p-society/raag/internal/metadata"
)

func TestBuildTransferMetadata(t *testing.T) {
	song := metadata.Song{
		Title:  "Song",
		Artist: "Artist",
		Album:  "Album",
		Path:   "/tmp/test.flac",
	}

	meta := buildTransferMetadata(song, 1234, "deadbeef", "", "")
	if meta.Version != constants.ShareProtocolVersion {
		t.Fatalf("version = %q, want %q", meta.Version, constants.ShareProtocolVersion)
	}
	if meta.Extension != ".flac" {
		t.Fatalf("extension = %q, want .flac", meta.Extension)
	}
	if meta.SizeBytes != 1234 {
		t.Fatalf("size = %d, want 1234", meta.SizeBytes)
	}
	if meta.SHA256 != "deadbeef" {
		t.Fatalf("sha256 = %q, want deadbeef", meta.SHA256)
	}
}

func TestTransferMetadataJSONRoundTrip(t *testing.T) {
	meta := transferMetadata{
		Version:   constants.ShareProtocolVersion,
		Title:     "Song",
		Artist:    "Artist",
		Album:     "Album",
		Filename:  "song.mp3",
		Extension: ".mp3",
		SizeBytes: 42,
		SHA256:    "abc123",
	}

	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("expected non-empty metadata payload")
	}

	var decoded transferMetadata
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Title != meta.Title || decoded.Extension != meta.Extension || decoded.SizeBytes != meta.SizeBytes {
		t.Fatalf("decoded metadata mismatch: got %#v want %#v", decoded, meta)
	}
}

func TestSHA256MatchesExpectedDigest(t *testing.T) {
	payload := []byte("raag-transfer-test")
	sum := sha256.Sum256(payload)
	got := hex.EncodeToString(sum[:])
	want := "3e4b621afbaa1fa1855aa6d938bbfdc790f6e6d43d12db695fcd6bf0385c0d2d"

	h := sha256.New()
	if _, err := h.Write(payload); err != nil {
		t.Fatalf("hash write: %v", err)
	}
	if digest := hex.EncodeToString(h.Sum(nil)); digest != got || digest != want {
		t.Fatalf("digest = %q, want %q", digest, want)
	}
}

func TestSanitizeTransferName(t *testing.T) {
	got := sanitizeTransferName("a/b\\c")
	if got != "a_b_c" {
		t.Fatalf("sanitizeTransferName() = %q, want %q", got, "a_b_c")
	}
	if sanitizeTransferName("   ") != "received" {
		t.Fatalf("expected blank transfer name to fall back to received")
	}
}

func TestMetadataPayloadFitsLimit(t *testing.T) {
	meta := transferMetadata{Title: string(bytes.Repeat([]byte("a"), 1024))}
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(raw) >= constants.TransferMaxMetadataSize {
		t.Fatalf("metadata payload unexpectedly exceeds limit: %d", len(raw))
	}
}
