package stream

import (
	"bytes"
	"context"
	"encoding/binary"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) Bytes() []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]byte(nil), l.b.Bytes()...)
}

func TestFeedRunsInRealTimeAndPadsWithSilence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan []byte, 1)
	var out lockedBuffer
	done := make(chan struct{})
	go func() { Feed(ctx, &out, in); close(done) }()

	loud := bytes.Repeat([]byte{0x7f}, 4000)
	in <- loud
	time.Sleep(500 * time.Millisecond)
	cancel()
	<-done

	got := out.Bytes()
	want := bytesPerSecond / 2
	if len(got) < want*8/10 || len(got) > want*12/10 {
		t.Errorf("wrote %d bytes in 0.5 s, want about %d", len(got), want)
	}
	if len(got)%4 != 0 {
		t.Errorf("output is not whole frames: %d bytes", len(got))
	}
	if !bytes.Contains(got, loud) {
		t.Error("queued audio was not passed through")
	}
	if !bytes.Contains(got, make([]byte, 1000)) {
		t.Error("gaps were not filled with silence")
	}
}

func TestFlacHeaderLen(t *testing.T) {
	// "fLaC", STREAMINFO (34 bytes, not last), then a 10 byte comment block marked last.
	h := []byte("fLaC")
	h = append(h, 0x00, 0, 0, 34)
	h = append(h, make([]byte, 34)...)
	h = append(h, 0x84, 0, 0, 10)
	h = append(h, make([]byte, 10)...)
	if n, ok := flacHeaderLen(append(h, 0xff, 0xf8)); !ok || n != len(h) {
		t.Errorf("flacHeaderLen = %d, %v; want %d, true", n, ok, len(h))
	}
	if _, ok := flacHeaderLen(h[:20]); ok {
		t.Error("a truncated header must not be reported complete")
	}
}

func TestWavHeaderLen(t *testing.T) {
	h := []byte("RIFF\xff\xff\xff\xffWAVE")
	h = append(h, "fmt "...)
	h = binary.LittleEndian.AppendUint32(h, 16)
	h = append(h, make([]byte, 16)...)
	h = append(h, "LIST"...) // ffmpeg adds an info chunk before the data
	h = binary.LittleEndian.AppendUint32(h, 6)
	h = append(h, make([]byte, 6)...)
	h = append(h, "data"...)
	h = binary.LittleEndian.AppendUint32(h, 0xffffffff)
	if n, ok := wavHeaderLen(append(h, 1, 2, 3, 4)); !ok || n != len(h) {
		t.Errorf("wavHeaderLen = %d, %v; want %d, true", n, ok, len(h))
	}
	if _, ok := wavHeaderLen(h[:30]); ok {
		t.Error("a truncated header must not be reported complete")
	}
}

func TestFormatLookup(t *testing.T) {
	if FormatByName("nonsense").Name != Formats[0].Name || FormatByExt("xyz").Name != Formats[0].Name {
		t.Error("unknown formats must fall back to the default")
	}
	if FormatByName("flac").Ext != "flac" || FormatByExt("wav").Name != "wav" {
		t.Error("lookup by name or extension failed")
	}
}
