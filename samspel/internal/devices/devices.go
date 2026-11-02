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
