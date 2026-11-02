package stream

import (
	"context"
	"io"
	"time"
)

const bytesPerSecond = SampleRate * Channels * 2

// Feed writes PCM from in to w at exactly real-time speed, padding with silence whenever
// nothing is queued, so that the encoder (and the speakers behind it) never run dry.
func Feed(ctx context.Context, w io.Writer, in <-chan []byte) {
	const maxQueue = 3 * bytesPerSecond // never fall further than 3 s behind
	var queue []byte
	start := time.Now()
	sent := 0
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case chunk := <-in:
			queue = append(queue, chunk...)
			if len(queue) > maxQueue {
				queue = queue[len(queue)-maxQueue:]
			}
		case <-tick.C:
			due := int(time.Since(start).Seconds()*bytesPerSecond)/4*4 - sent
			if due <= 0 {
				continue
			}
			out := make([]byte, due)
			n := min(len(queue)/4*4, due)
			copy(out, queue[:n])
			queue = queue[n:]
			if _, err := w.Write(out); err != nil {
				return
			}
			sent += due
		}
	}
}
