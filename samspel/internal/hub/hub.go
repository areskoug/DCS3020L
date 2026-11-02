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
	"strings"
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
	// DelaysMs holds the extra delay per device, used to line up speakers that buffer differently.
	// Speakers buffer each format differently, so see DelayKey for how the entries are keyed.
	DelaysMs map[string]int `json:"delaysMs,omitempty"`
	// DefaultDelaysMs holds the delays used for devices that have no entry in DelaysMs. Not stored.
	DefaultDelaysMs map[string]int `json:"defaultDelaysMs,omitempty"`
	// Format is the stream encoding: "mp3-192" (default, empty) or "wav" (uncompressed, for lower lag).
	Format string `json:"format,omitempty"`
	Active bool   `json:"active"`
	// TrimsPct scales the group volume per device (default 100), so one room can stay quieter than another.
	TrimsPct map[string]int `json:"trimsPct,omitempty"`
	// Solo marks the implicit one-speaker group every device gets, so it can be picked on its own in AirPlay.
	// Solo groups are rebuilt from the device list and never stored.
	Solo bool `json:"solo,omitempty"`
	// AirPlay tells the UI whether the group is visible as an AirPlay speaker. Not stored.
	AirPlay bool `json:"airplay"`
	// NowPlaying is what the AirPlay source reports. Not stored.
	NowPlaying *airplay.NowPlaying `json:"nowPlaying,omitempty"`
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
	lastVol    map[string]int  // the last group volume, reapplied when the balance changes
}

type running struct {
	b      *stream.Broadcaster
	cancel context.CancelFunc
}

func New(port int, statePath string, air *airplay.Manager) *Hub {
	h := &Hub{port: port, statePath: statePath, air: air, devices: map[string]devices.Device{}, groups: map[string]*Group{}, streams: map[string]*running{}, pendingVol: map[string]int{}, volBusy: map[string]bool{}, lastVol: map[string]int{}}
	h.load()
	h.mu.Lock()
	h.rebuildSolos()
	h.mu.Unlock()
	h.syncAirPlay()
	return h
}

const soloPrefix = "solo-"

// mixedHEOSDelayMs is added by default to a HEOS speaker in a group that also holds a Cast speaker: with the
// same lead-in HEOS played slightly early, and 50 ms put them in step.
const mixedHEOSDelayMs = 50

// delaysFor returns the delay to use per DelayKey: the defaults, overridden by every delay set on the group
// (an explicit 0 counts). It must be called with h.mu held.
func (h *Hub) delaysFor(g *Group, format string) map[string]int {
	out := map[string]int{}
	var haveCast bool
	var heos []string
	for _, id := range g.DeviceIDs {
		switch h.devices[id].Kind {
		case devices.Cast:
			haveCast = true
		case devices.HEOS:
			heos = append(heos, id)
		}
	}
	if haveCast {
		for _, id := range heos {
			out[DelayKey(id, format)] = mixedHEOSDelayMs
		}
	}
	return out
}

// effectiveDelays is delaysFor with the group's own delays applied on top.
func (h *Hub) effectiveDelays(g *Group, format string) map[string]int {
	out := h.delaysFor(g, format)
	for k, v := range g.DelaysMs {
		out[k] = v
	}
	return out
}

// DelayKey is the DelaysMs key for a device. The default format uses the plain device ID, others add "@format",
// so delays calibrated for one format are kept when switching to another and back.
func DelayKey(deviceID, format string) string {
	if format == "" || format == stream.Formats[0].Name {
		return deviceID
	}
	return deviceID + "@" + format
}

// rebuildSolos makes one single-speaker group per known device. It must be called with h.mu held.
// A device whose name equals a user group's name gets a suffix, because AirPlay names must be unique.
func (h *Hub) rebuildSolos() {
	taken := map[string]bool{}
	for id, g := range h.groups {
		if !g.Solo {
			taken[g.Name] = true
		} else if _, ok := h.devices[strings.TrimPrefix(id, soloPrefix)]; !ok {
			delete(h.groups, id)
		}
	}
	for _, d := range h.devices {
		name := d.Name
		if taken[name] {
			name += " (högtalare)"
		}
		id := soloPrefix + d.ID
		if g, ok := h.groups[id]; ok {
			g.Name, g.DeviceIDs, g.Format = name, []string{d.ID}, d.Format
			continue
		}
		h.groups[id] = &Group{ID: id, Name: name, DeviceIDs: []string{d.ID}, Format: d.Format, Solo: true}
	}
}

// Run keeps the device list fresh: once at startup and then every ten minutes.
func (h *Hub) Run(ctx context.Context) {
	for {
		if err := h.Discover(ctx); err != nil {
			log.Printf("discover: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Minute):
		}
	}
}

func (h *Hub) devicesPath() string { return strings.TrimSuffix(h.statePath, ".json") + ".devices.json" }

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
	h.loadDevices()
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

func (h *Hub) loadDevices() {
	var ds []devices.Device
	if data, err := os.ReadFile(h.devicesPath()); err == nil && json.Unmarshal(data, &ds) == nil {
		for _, d := range ds {
			h.devices[d.ID] = d
		}
	}
}

// save must be called with h.mu held.
func (h *Hub) save() {
	gs := make([]*Group, 0, len(h.groups))
	for _, g := range h.groups {
		if !g.Solo {
			gs = append(gs, g)
		}
	}
	data, _ := json.MarshalIndent(gs, "", "  ")
	if err := os.WriteFile(h.statePath, data, 0o644); err != nil {
		log.Printf("save state: %v", err)
	}
	ds := make([]devices.Device, 0, len(h.devices))
	for _, d := range h.devices {
		ds = append(ds, d)
	}
	data, _ = json.MarshalIndent(ds, "", "  ")
	if err := os.WriteFile(h.devicesPath(), data, 0o644); err != nil {
		log.Printf("save devices: %v", err)
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
		d.Format, d.CushionSec = h.devices[d.ID].Format, h.devices[d.ID].CushionSec
		h.devices[d.ID] = d
	}
	h.rebuildSolos()
	h.save()
	h.mu.Unlock()
	h.syncAirPlay()
	return errors.Join(errs...)
}

func (h *Hub) Devices() []devices.Device {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]devices.Device, 0, len(h.devices))
	for _, d := range h.devices {
		d.CushionSec = d.Cushion() // the value in effect, also when it is the default
		out = append(out, d)
	}
	return out
}

// SetDeviceFormat sets the stream format used when a speaker is played on its own.
// It reports false if the device or the format is unknown. It takes effect the next time the speaker starts.
func (h *Hub) SetDeviceFormat(id, format string) bool {
	if format != "" && stream.FormatByName(format).Name != format {
		return false
	}
	if format == stream.Formats[0].Name {
		format = ""
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.devices[id]
	if !ok {
		return false
	}
	d.Format = format
	h.devices[id] = d
	h.rebuildSolos()
	h.save()
	return true
}

// SetDeviceCushion sets how many seconds of silence a speaker gets up front (1 to 30).
func (h *Hub) SetDeviceCushion(id string, seconds int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.devices[id]
	if !ok || seconds < 1 || seconds > 30 {
		return false
	}
	if seconds == devices.DefaultCushionSec {
		seconds = 0
	}
	d.CushionSec = seconds
	h.devices[id] = d
	h.save()
	return true
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
		if !g.Solo {
			c.DefaultDelaysMs = h.delaysFor(g, stream.FormatByName(g.Format).Name)
			if len(c.DefaultDelaysMs) == 0 {
				c.DefaultDelaysMs = nil
			}
		}
		_, c.AirPlay = h.air.PCM(g.ID)
		if np, ok := h.air.NowPlaying(g.ID); ok && (np.Playing || np.Title != "") {
			c.NowPlaying = &np
		}
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
		if old.Solo {
			h.mu.Unlock()
			return *old
		}
		g.Active = old.Active
	}
	g.Solo = false
	g.AirPlay, g.NowPlaying, g.DefaultDelaysMs = false, nil, nil // computed on read, never stored
	h.groups[g.ID] = &g
	h.rebuildSolos()
	h.save()
	vol, haveVol := h.lastVol[g.ID]
	h.mu.Unlock()
	h.syncAirPlay()
	if haveVol && g.Active {
		h.SetGroupVolume(g.ID, vol) // a changed balance should be heard right away
	}
	return g
}

func (h *Hub) DeleteGroup(ctx context.Context, id string) {
	h.mu.Lock()
	g, ok := h.groups[id]
	h.mu.Unlock()
	if !ok || g.Solo {
		return
	}
	h.StopGroup(ctx, id)
	h.mu.Lock()
	delete(h.groups, id)
	h.rebuildSolos()
	h.save()
	h.mu.Unlock()
	h.syncAirPlay()
}

// Subscribe attaches a listener to a running group's stream.
func (h *Hub) Subscribe(id string) (header []byte, ch <-chan []byte, unsubscribe func(), ok bool) {
	h.mu.Lock()
	r, ok := h.streams[id]
	h.mu.Unlock()
	if !ok {
		return nil, nil, nil, false
	}
	header, ch, unsubscribe = r.b.Subscribe()
	return header, ch, unsubscribe, true
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
	format := stream.FormatByName(g.Format)
	b, err := stream.New(sctx, format)
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
	delays := h.effectiveDelays(g, format.Name)
	h.mu.Unlock()

	var errs []error
	for _, did := range members {
		d, ok := h.Device(did)
		if !ok {
			errs = append(errs, fmt.Errorf("device %s not found", did))
			continue
		}
		lead := delays[DelayKey(did, format.Name)] + d.Cushion()*1000
		url, err := h.streamURL(d, id, lead, format.Ext)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := devices.NewPlayer(d).Play(ctx, url, format.ContentType); err != nil {
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
func (h *Hub) streamURL(d devices.Device, groupID string, leadMs int, ext string) (string, error) {
	c, err := net.Dial("udp4", net.JoinHostPort(d.Addr, "9"))
	if err != nil {
		return "", err
	}
	defer c.Close()
	ip := c.LocalAddr().(*net.UDPAddr).IP
	return fmt.Sprintf("http://%s:%d/stream/%s/%d.%s", ip, h.port, groupID, leadMs, ext), nil
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
	h.lastVol[id] = percent
	h.pendingVol[id] = percent
	if h.volBusy[id] {
		h.mu.Unlock()
		return
	}
	h.volBusy[id] = true
	members := append([]string(nil), g.DeviceIDs...)
	trims := make(map[string]int, len(g.TrimsPct))
	for k, v := range g.TrimsPct {
		trims[k] = v
	}
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
				level := p
				if t, ok := trims[did]; ok {
					level = p * max(0, min(100, t)) / 100
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := devices.NewPlayer(d).SetVolume(context.Background(), level); err != nil {
						log.Printf("volume %s: %v", d.Name, err)
						return
					}
					log.Printf("volume %s: set to %d%%", d.Name, level)
				}()
			}
			wg.Wait()
		}
	}()
}

// Cover returns the current cover art for a group, or nil.
func (h *Hub) Cover(id string) []byte { return h.air.Cover(id) }
