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

	"github.com/areskoug/dcs3020l/internal/devices"
	"github.com/areskoug/dcs3020l/internal/stream"
)

type Group struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	DeviceIDs []string `json:"deviceIds"`
	Active    bool     `json:"active"`
}

type Hub struct {
	port      int
	statePath string

	mu      sync.Mutex
	devices map[string]devices.Device
	groups  map[string]*Group
	streams map[string]*running
}

type running struct {
	b      *stream.Broadcaster
	cancel context.CancelFunc
}

func New(port int, statePath string) *Hub {
	h := &Hub{port: port, statePath: statePath, devices: map[string]devices.Device{}, groups: map[string]*Group{}, streams: map[string]*running{}}
	h.load()
	return h
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
		out = append(out, *g)
	}
	return out
}

// SaveGroup creates or updates a group. An empty ID creates a new one.
func (h *Hub) SaveGroup(g Group) Group {
	h.mu.Lock()
	defer h.mu.Unlock()
	if g.ID == "" {
		g.ID = fmt.Sprintf("g%d", time.Now().UnixNano())
	}
	if old, ok := h.groups[g.ID]; ok {
		g.Active = old.Active
	}
	h.groups[g.ID] = &g
	h.save()
	return g
}

func (h *Hub) DeleteGroup(ctx context.Context, id string) {
	h.StopGroup(ctx, id)
	h.mu.Lock()
	delete(h.groups, id)
	h.save()
	h.mu.Unlock()
}

// Subscribe attaches a listener to a running group's stream.
func (h *Hub) Subscribe(id string) (<-chan []byte, func(), bool) {
	h.mu.Lock()
	r, ok := h.streams[id]
	h.mu.Unlock()
	if !ok {
		return nil, nil, false
	}
	ch, unsub := r.b.Subscribe()
	return ch, unsub, true
}

// StartGroup starts the group's stream (a test tone until the AirPlay source exists)
// and points every member device at it.
func (h *Hub) StartGroup(ctx context.Context, id string) error {
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
	go func() {
		if err := stream.Tone(sctx, b); err != nil && sctx.Err() == nil {
			log.Printf("tone source: %v", err)
		}
	}()

	h.mu.Lock()
	h.streams[id] = &running{b: b, cancel: cancel}
	g.Active = true
	members := append([]string(nil), g.DeviceIDs...)
	h.mu.Unlock()

	var errs []error
	for _, did := range members {
		d, ok := h.Device(did)
		if !ok {
			errs = append(errs, fmt.Errorf("device %s not found", did))
			continue
		}
		url, err := h.streamURL(d, id)
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
func (h *Hub) streamURL(d devices.Device, groupID string) (string, error) {
	c, err := net.Dial("udp4", net.JoinHostPort(d.Addr, "9"))
	if err != nil {
		return "", err
	}
	defer c.Close()
	ip := c.LocalAddr().(*net.UDPAddr).IP
	return fmt.Sprintf("http://%s:%d/stream/%s.mp3", ip, h.port, groupID), nil
}
