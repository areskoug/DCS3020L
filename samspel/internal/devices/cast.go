package devices

import (
	"context"
	"fmt"
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

func (p *castPlayer) Play(_ context.Context, url, contentType string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	app, err := p.connect()
	if err != nil {
		return err
	}
	defer app.Close(false)
	// detach=true returns right after the load command instead of waiting for the media to end.
	return app.Load(url, 0, contentType, false, true, false)
}

func (p *castPlayer) Stop(_ context.Context) error {
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
