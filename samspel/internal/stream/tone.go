package stream

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

// Tone writes a looping test tone as PCM to w until ctx is cancelled.
func Tone(ctx context.Context, w io.Writer) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-re",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100",
		"-ac", fmt.Sprint(Channels), "-f", "s16le", "pipe:1")
	cmd.Stdout = w
	return cmd.Run()
}
