package stream

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

// Tone writes a test signal as PCM: every 8 seconds a pair of beeps followed by a single beep. The pattern does
// not repeat every second, so two speakers that are whole seconds apart are heard as apart, which a plain
// once-a-second beep cannot show. to w until ctx is cancelled.
func Tone(ctx context.Context, w io.Writer) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-re",
		"-f", "lavfi", "-i", "sine=frequency=1000:sample_rate=44100",
		"-af", "volume='if(lt(mod(t,8),0.1)+between(mod(t,8),0.5,0.6)+between(mod(t,8),2.5,2.6),1,0)':eval=frame",
		"-ac", fmt.Sprint(Channels), "-f", "s16le", "pipe:1")
	cmd.Stdout = w
	return cmd.Run()
}
