// Package hub keeps the device list and speaker groups and runs a stream per active group.
package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/areskoug/dcs3020l/internal/airplay"
	"github.com/areskoug/dcs3020l/internal/devices"
	"github.com/areskoug/dcs3020l/internal/stream"
)

type Group struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	DeviceIDs []string `json:"deviceIds"`
	// DelaysMs holds the extra delay per device ID, used to line up speakers that buffer differently.
	DelaysMs map[string]int `json:"delaysMs,omitempty"`
	Active   bool           `json:"active"`
	// AirPlay tells the UI whether the group is visible as an AirPlay speaker. Not stored.
	AirPlay bool `json:"airplay"`
}

type Hub struct {
	port      int
	statePath string
	air       *airplay.Manager // nil when shairport-sync is unavailable

	mu      sync.Mutex
	devices map[string]devices.Device
	groups  map[string]*Group
	streams map[string]*running

	pendingVol map[string]int  // latest requested group volume, not yet applied
	volBusy    map[string]bool // a goroutine is applying volumes for this group
}

type running struct {
	b      *stream.Broadcaster
	cancel context.CancelFunc
}

func New(port int, statePath string, air *airplay.Manager) *Hub {
	h := &Hub{port: port, statePath: statePath, air: air, devices: map[string]devices.Device{}, groups: map[string]*Group{}, streams: map[string]*running{}, pendingVol: map[string]int{}, volBusy: map[string]bool{}}
	h.load()
	h.syncAirPlay()
	return h
}

// syncAirPlay publishes the current groups as AirPlay speakers.
func (h *Hub) syncAirPlay() {
	h.mu.Lock()
	gs := make([]airplay.Group, 0, len(h.groups))
	for _, g := range h.groups {
		gs = append(gs, airplay.Group{ID: g.ID, Name: g.Name})
	}
	h.mu.Unlock()
	h.air.Sync(gs)
}

func (h *Hub) load() {
	data, err := os.ReadFile(h.statePath)
	if err != nil {
		return
	}
	var gs []*Group
	if json.Unmarshal(data, &gs) != nil {
		return
	}
	for _, g := range gs {
		g.Active = false
		h.groups[g.ID] = g
	}
}

// save must be called with h.mu held.
func (h *Hub) save() {
	gs := make([]*Group, 0, len(h.groups))
	for _, g := range h.groups {
		gs = append(gs, g)
	}
	data, _ := json.MarshalIndent(gs, "", "  ")
	if err := os.WriteFile(h.statePath, data, 0o644); err != nil {
		log.Printf("save state: %v", err)
	}
}

// Discover refreshes the device list from Cast and HEOS.
func (h *Hub) Discover(ctx context.Context) error {
	var found []devices.Device
	var errs []error
	if ds, err := devices.DiscoverCast(ctx, 4*time.Second); err != nil {
		errs = append(errs, fmt.Errorf("cast: %w", err))
	} else {
		found = append(found, ds...)
	}
	if ds, err := devices.DiscoverHEOS(ctx, 4*time.Second); err != nil {
		errs = append(errs, fmt.Errorf("heos: %w", err))
	} else {
		found = append(found, ds...)
	}
	h.mu.Lock()
	for _, d := range found {
		h.devices[d.ID] = d
	}
	h.mu.Unlock()
	return errors.Join(errs...)
}

func (h *Hub) Devices() []devices.Device {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]devices.Device, 0, len(h.devices))
	for _, d := range h.devices {
		out = append(out, d)
	}
	return out
}

func (h *Hub) Device(id string) (devices.Device, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.devices[id]
	return d, ok
}

func (h *Hub) Groups() []Group {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Group, 0, len(h.groups))
	for _, g := range h.groups {
		c := *g
		_, c.AirPlay = h.air.PCM(g.ID)
		out = append(out, c)
	}
	return out
}

// SaveGroup creates or updates a group. An empty ID creates a new one.
func (h *Hub) SaveGroup(g Group) Group {
	h.mu.Lock()
	if g.ID == "" {
		g.ID = fmt.Sprintf("g%d", time.Now().UnixNano())
	}
	if old, ok := h.groups[g.ID]; ok {
		g.Active = old.Active
	}
	h.groups[g.ID] = &g
	h.save()
	h.mu.Unlock()
	h.syncAirPlay()
	return g
}

func (h *Hub) DeleteGroup(ctx context.Context, id string) {
	h.StopGroup(ctx, id)
	h.mu.Lock()
	delete(h.groups, id)
	h.save()
	h.mu.Unlock()
	h.syncAirPlay()
}

// Subscribe attaches a listener to a running group's stream.
func (h *Hub) Subscribe(id string) (<-chan stream.Chunk, func(), bool) {
	h.mu.Lock()
	r, ok := h.streams[id]
	h.mu.Unlock()
	if !ok {
		return nil, nil, false
	}
	ch, unsub := r.b.Subscribe()
	return ch, unsub, true
}

// StartGroup starts the group's stream and points every member device at it. The audio comes from the
// group's AirPlay receiver, or from a test beep if tone is set or AirPlay is unavailable.
func (h *Hub) StartGroup(ctx context.Context, id string, tone bool) error {
	h.mu.Lock()
	g, ok := h.groups[id]
	if !ok {
		h.mu.Unlock()
		return fmt.Errorf("unknown group %q", id)
	}
	if _, running := h.streams[id]; running {
		h.mu.Unlock()
		return nil
	}
	h.mu.Unlock()

	sctx, cancel := context.WithCancel(context.Background())
	b, err := stream.New(sctx)
	if err != nil {
		cancel()
		return err
	}
	if pcm, ok := h.air.PCM(id); ok && !tone {
		go stream.Feed(sctx, b, pcm)
	} else {
		go func() {
			if err := stream.Tone(sctx, b); err != nil && sctx.Err() == nil {
				log.Printf("tone source: %v", err)
			}
		}()
	}

	h.mu.Lock()
	h.streams[id] = &running{b: b, cancel: cancel}
	g.Active = true
	members := append([]string(nil), g.DeviceIDs...)
	delays := make(map[string]int, len(g.DelaysMs))
	for k, v := range g.DelaysMs {
		delays[k] = v
	}
	h.mu.Unlock()

	var errs []error
	for _, did := range members {
		d, ok := h.Device(did)
		if !ok {
			errs = append(errs, fmt.Errorf("device %s not found", did))
			continue
		}
		url, err := h.streamURL(d, id, delays[did])
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := devices.NewPlayer(d).Play(ctx, url); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", d.Name, err))
		}
	}
	return errors.Join(errs...)
}

func (h *Hub) StopGroup(ctx context.Context, id string) {
	h.mu.Lock()
	g := h.groups[id]
	r := h.streams[id]
	delete(h.streams, id)
	var members []string
	if g != nil {
		g.Active = false
		members = append(members, g.DeviceIDs...)
	}
	h.mu.Unlock()
	if r == nil {
		return
	}
	for _, did := range members {
		if d, ok := h.Device(did); ok {
			if err := devices.NewPlayer(d).Stop(ctx); err != nil {
				log.Printf("stop %s: %v", d.Name, err)
			}
		}
	}
	r.cancel()
}

// streamURL builds the URL the device should fetch, using the local IP that routes to it.
func (h *Hub) streamURL(d devices.Device, groupID string, delayMs int) (string, error) {
	c, err := net.Dial("udp4", net.JoinHostPort(d.Addr, "9"))
	if err != nil {
		return "", err
	}
	defer c.Close()
	ip := c.LocalAddr().(*net.UDPAddr).IP
	return fmt.Sprintf("http://%s:%d/stream/%s/%d.mp3", ip, h.port, groupID, delayMs), nil
}

// SetGroupVolume sets the volume of every device in the group. Requests that arrive while speakers are
// still being updated are coalesced, so dragging the phone's slider only applies the latest value.
func (h *Hub) SetGroupVolume(id string, percent int) {
	h.mu.Lock()
	g, ok := h.groups[id]
	if !ok {
		h.mu.Unlock()
		return
	}
	log.Printf("volume %s: requested %d%%", g.Name, percent)
	h.pendingVol[id] = percent
	if h.volBusy[id] {
		h.mu.Unlock()
		return
	}
	h.volBusy[id] = true
	members := append([]string(nil), g.DeviceIDs...)
	h.mu.Unlock()

	go func() {
		for {
			h.mu.Lock()
			p, more := h.pendingVol[id]
			delete(h.pendingVol, id)
			if !more {
				h.volBusy[id] = false
				h.mu.Unlock()
				return
			}
			h.mu.Unlock()
			var wg sync.WaitGroup
			for _, did := range members {
				d, ok := h.Device(did)
				if !ok {
					continue
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := devices.NewPlayer(d).SetVolume(context.Background(), p); err != nil {
						log.Printf("volume %s: %v", d.Name, err)
						return
					}
					log.Printf("volume %s: set to %d%%", d.Name, p)
				}()
			}
			wg.Wait()
		}
	}()
}
