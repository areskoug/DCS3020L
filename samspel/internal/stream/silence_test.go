package stream

import (
	"bytes"
	"os/exec"
	"testing"
	"time"
)

func needFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
}

func TestSilenceWavIsExactlyZeroedPCMWithoutHeader(t *testing.T) {
	needFFmpeg(t)
	b, err := Silence(FormatByName("wav"), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if want := 2 * bytesPerSecond; len(b) != want {
		t.Errorf("got %d bytes, want %d", len(b), want)
	}
	if !bytes.Equal(b, make([]byte, len(b))) {
		t.Error("silence must be all zeros")
	}
}

func TestSilenceMp3HasTheRightLength(t *testing.T) {
	needFFmpeg(t)
	b, err := Silence(FormatByName("mp3-192"), 4*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if want := 4 * 192000 / 8; len(b) < want*9/10 || len(b) > want*11/10 {
		t.Errorf("got %d bytes, want about %d", len(b), want)
	}
	if b[0] != 0xff && b[0] != 'I' { // an MP3 frame sync or an ID3 tag
		t.Errorf("does not look like MP3: % x", b[:4])
	}
}

func TestSilenceIsCached(t *testing.T) {
	needFFmpeg(t)
	a, _ := Silence(FormatByName("mp3-192"), 1500*time.Millisecond)
	b, _ := Silence(FormatByName("mp3-192"), 1500*time.Millisecond)
	if len(a) == 0 || &a[0] != &b[0] {
		t.Error("the second call should return the cached bytes")
	}
}
