// Package devices discovers speakers on the network and tells them to play a stream URL.
package devices

import (
	"context"
	"time"
)

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
	// CatchUpSec makes a Cast speaker skip ahead once it has started, keeping only this many seconds of
	// buffer, to cut the lag (0 = off). Experimental, and a setting that discovery must keep.
	CatchUpSec int `json:"catchUpSec,omitempty"`
}

// PlayOptions describes how a device should play a stream.
type PlayOptions struct {
	ContentType string
	// Delay is how late the stream's audio starts arriving at this device.
	Delay time.Duration
}

// Player controls a single device.
type Player interface {
	Play(ctx context.Context, url string, o PlayOptions) error
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
