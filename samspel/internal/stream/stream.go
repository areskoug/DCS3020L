// Package stream turns raw PCM into an live audio stream and fans it out to HTTP clients.
package stream

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

const (
	SampleRate = 44100
	Channels   = 2
)

// Chunk is a piece of encoded audio and the moment it left the encoder.
type Chunk struct {
	At   time.Time
	Data []byte
}

// Broadcaster encodes PCM (s16le, 44.1 kHz, stereo) written to it and copies the
// encoded bytes to every subscribed listener. Slow listeners drop chunks rather
// than stalling the others.
type Broadcaster struct {
	mu   sync.Mutex
	subs map[chan Chunk]struct{}

	pcmIn io.WriteCloser
	cmd   *exec.Cmd

	headerLen func([]byte) (int, bool)
	pending   []byte // bytes seen before the header was complete
	header    []byte // set once complete
}

// New starts an ffmpeg process that encodes PCM from Write into the given format.
func New(ctx context.Context, f Format) (*Broadcaster, error) {
	args := []string{"-hide_banner", "-loglevel", "error",
		"-f", "s16le", "-ar", fmt.Sprint(SampleRate), "-ac", fmt.Sprint(Channels), "-i", "pipe:0"}
	args = append(append(args, f.Args...), "-flush_packets", "1", "pipe:1")
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}
	b := &Broadcaster{subs: map[chan Chunk]struct{}{}, pcmIn: in, cmd: cmd, headerLen: f.HeaderLen}
	go b.pump(out)
	return b, nil
}

func (b *Broadcaster) pump(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := Chunk{At: time.Now(), Data: append([]byte(nil), buf[:n]...)}
			b.mu.Lock()
			if b.headerLen != nil && b.header == nil {
				b.pending = append(b.pending, chunk.Data...)
				hl, ok := b.headerLen(b.pending)
				if !ok {
					b.mu.Unlock()
					continue
				}
				b.header = append([]byte(nil), b.pending[:hl]...)
				chunk.Data = b.pending
				b.pending = nil
			}
			for ch := range b.subs {
				select {
				case ch <- chunk:
				default:
				}
			}
			b.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// Write feeds PCM into the encoder.
func (b *Broadcaster) Write(p []byte) (int, error) { return b.pcmIn.Write(p) }

// Close stops the encoder.
func (b *Broadcaster) Close() error {
	b.pcmIn.Close()
	return b.cmd.Wait()
}

// Subscribe returns a channel of encoded chunks and a function to unsubscribe.
// The buffer holds about 40 s of audio so that listeners can delay playback.
func (b *Broadcaster) Subscribe() (<-chan Chunk, func()) {
	ch := make(chan Chunk, 1024)
	b.mu.Lock()
	if b.header != nil {
		ch <- Chunk{Data: b.header} // zero time: due immediately
	}
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}
