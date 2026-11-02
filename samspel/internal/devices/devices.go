// Package devices discovers speakers on the network and tells them to play a stream URL.
package devices

import "context"

type Kind string

const (
	Cast Kind = "cast"
	HEOS Kind = "heos"
)

// Device is a discovered speaker.
type Device struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind Kind   `json:"kind"`
	Addr string `json:"addr"`
	Port int    `json:"port"`
	// HEOSPid is the HEOS player id, set only for Kind == HEOS.
	HEOSPid string `json:"heosPid,omitempty"`
	// Format is the stream encoding used when this speaker is played on its own ("" is the default).
	// It is a setting, not discovery data, so discovery must keep it.
	Format string `json:"format,omitempty"`
	// CushionSec is how many seconds of silence the speaker gets up front, so it starts with audio in hand
	// (0 means DefaultCushion for its kind). A setting, so discovery must keep it.
	CushionSec int `json:"cushionSec,omitempty"`
}

// DefaultCushion is the cushion, in seconds, for a device with none set. A Cast speaker that gets only 3 s
// sometimes stalls at startup, while 4 s and up was reliable; HEOS starts cleanly on 3 s.
func DefaultCushion(k Kind) int {
	if k == Cast {
		return 5
	}
	return 3
}

// Cushion returns the silence, in seconds, to send ahead of the live audio.
func (d Device) Cushion() int {
	if d.CushionSec <= 0 {
		return DefaultCushion(d.Kind)
	}
	return d.CushionSec
}

// Player controls a single device.
type Player interface {
	Play(ctx context.Context, url, contentType string) error
	Stop(ctx context.Context) error
	SetVolume(ctx context.Context, percent int) error
}

// NewPlayer returns the Player for a device.
func NewPlayer(d Device) Player {
	switch d.Kind {
	case HEOS:
		return &heosPlayer{dev: d}
	default:
		return &castPlayer{dev: d}
	}
}
