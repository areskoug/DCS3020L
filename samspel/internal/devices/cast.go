package devices

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/vishen/go-chromecast/application"
	"github.com/vishen/go-chromecast/dns"
)

// DiscoverCast browses mDNS for Google Cast devices for the given duration.
func DiscoverCast(ctx context.Context, d time.Duration) ([]Device, error) {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	entries, err := dns.DiscoverCastDNSEntries(ctx, nil)
	if err != nil {
		return nil, err
	}
	var out []Device
	for {
		select {
		case e, ok := <-entries:
			if !ok {
				return out, nil
			}
			out = append(out, Device{ID: "cast-" + e.GetUUID(), Name: e.GetName(), Kind: Cast, Addr: e.GetAddr(), Port: e.GetPort()})
		case <-ctx.Done():
			return out, nil
		}
	}
}

type castPlayer struct {
	dev Device
	mu  sync.Mutex
}

func (p *castPlayer) connect() (*application.Application, error) {
	app := application.NewApplication(application.WithCacheDisabled(true))
	if err := app.Start(p.dev.Addr, p.dev.Port); err != nil {
		return nil, fmt.Errorf("connect %s: %w", p.dev.Name, err)
	}
	return app, nil
}

func (p *castPlayer) Play(_ context.Context, url string, o PlayOptions) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	app, err := p.connect()
	if err != nil {
		return err
	}
	defer app.Close(false)
	// detach=true returns right after the load command instead of waiting for the media to end.
	if err := app.Load(url, 0, o.ContentType, false, true, false); err != nil {
		return err
	}
	p.watch(time.Now().Add(o.Delay))
	return nil
}

var (
	watchMu sync.Mutex
	watches = map[string]context.CancelFunc{}
)

func stopWatch(id string) {
	watchMu.Lock()
	defer watchMu.Unlock()
	if cancel, ok := watches[id]; ok {
		cancel()
		delete(watches, id)
	}
}

// watch follows the speaker's player state for a while: it logs when it starts playing and, if the device
// has CatchUpSec set, skips ahead in the stream once it plays. firstAudio is when the stream's audio starts
// to arrive, so the seconds of content received so far are time.Since(firstAudio).
func (p *castPlayer) watch(firstAudio time.Time) {
	stopWatch(p.dev.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	watchMu.Lock()
	watches[p.dev.ID] = cancel
	watchMu.Unlock()
	go func() {
		defer cancel()
		started := time.Now()
		var seeked bool
		var lastState string
		for polls := 0; ctx.Err() == nil; polls++ {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			app := application.NewApplication(application.WithCacheDisabled(true))
			if err := app.Start(p.dev.Addr, p.dev.Port); err != nil {
				continue
			}
			_, media, _ := app.Status()
			if media == nil {
				app.Close(false)
				continue
			}
			state, pos := media.PlayerState, media.CurrentTime
			if state != lastState {
				log.Printf("cast %s: %s at %.1fs, %.1fs after the stream was loaded", p.dev.Name, state, pos, time.Since(started).Seconds())
				lastState = state
			}
			if state == "PLAYING" && p.dev.CatchUpSec > 0 && !seeked {
				received := time.Since(firstAudio).Seconds()
				target := received - float64(p.dev.CatchUpSec)
				if target > float64(pos)+2 {
					err := app.SeekToTime(float32(target))
					log.Printf("cast %s: skipping from %.1fs to %.1fs (received %.1fs, keeping %ds): %v", p.dev.Name, pos, target, received, p.dev.CatchUpSec, err)
					seeked = true
				}
			}
			app.Close(false)
			if state == "PLAYING" && (seeked || p.dev.CatchUpSec == 0) && polls > 4 {
				return
			}
		}
	}()
}

func (p *castPlayer) Stop(_ context.Context) error {
	stopWatch(p.dev.ID)
	p.mu.Lock()
	defer p.mu.Unlock()
	app, err := p.connect()
	if err != nil {
		return err
	}
	defer app.Close(false)
	return app.Stop()
}

func (p *castPlayer) SetVolume(_ context.Context, percent int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	app, err := p.connect()
	if err != nil {
		return err
	}
	defer app.Close(false)
	return app.SetVolume(float32(percent) / 100)
}
