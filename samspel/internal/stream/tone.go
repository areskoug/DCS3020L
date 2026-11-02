package stream

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

// Tone writes a short 1 kHz beep once a second as PCM (easy to hear whether speakers are in sync) to w until ctx is cancelled.
func Tone(ctx context.Context, w io.Writer) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-re",
		"-f", "lavfi", "-i", "sine=frequency=1000:sample_rate=44100",
		"-af", "volume='if(lt(mod(t,1),0.1),1,0)':eval=frame",
		"-ac", fmt.Sprint(Channels), "-f", "s16le", "pipe:1")
	cmd.Stdout = w
	return cmd.Run()
}
