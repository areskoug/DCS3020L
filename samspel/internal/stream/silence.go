package stream

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

var (
	silenceMu    sync.Mutex
	silenceCache = map[string][]byte{}
)

// Silence returns d of silence encoded in format f, without the stream header, so it can be sent in front of
// the live audio. A speaker that starts with a few seconds of audio already in hand does not run dry and stall.
func Silence(f Format, d time.Duration) ([]byte, error) {
	d = d.Round(10 * time.Millisecond)
	key := fmt.Sprintf("%s/%d", f.Name, d.Milliseconds())
	silenceMu.Lock()
	defer silenceMu.Unlock()
	if b, ok := silenceCache[key]; ok {
		return b, nil
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi",
		"-i", fmt.Sprintf("anullsrc=r=%d:cl=stereo", SampleRate), "-t", fmt.Sprintf("%.2f", d.Seconds())}
	args = append(append(args, f.Args...), "pipe:1")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("encode silence: %w", err)
	}
	b := out.Bytes()
	if f.HeaderLen != nil {
		n, ok := f.HeaderLen(b)
		if !ok {
			return nil, fmt.Errorf("encode silence: no %s header found", f.Name)
		}
		b = b[n:]
	}
	silenceCache[key] = b
	return b, nil
}
