// Package airplay runs one shairport-sync instance per group, so every group shows up as its own
// AirPlay speaker, and hands the received PCM to the hub.
package airplay

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	basePort = 5100
	// activeTimeout is how long shairport-sync waits after playback ends before the hub stops the group.
	activeTimeout = 20
)

// Group is what the manager needs to know about a hub group.
type Group struct{ ID, Name string }

type instance struct {
	name   string
	cancel context.CancelFunc
	pcm    chan []byte
}

// Manager keeps the shairport-sync processes in line with the hub's groups.
type Manager struct {
	ctx     context.Context
	bin     string
	hubBin  string
	dir     string
	hubPort int

	mu       sync.Mutex
	inst     map[string]*instance
	ports    map[string]int
	nextPort int
}

// New returns a Manager, or an error if the shairport-sync binary cannot be found.
// hubBin is the hub executable, which shairport-sync calls (as "notify") on play start and end.
func New(ctx context.Context, bin, hubBin, dir string, hubPort int) (*Manager, error) {
	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Manager{ctx: ctx, bin: path, hubBin: hubBin, dir: dir, hubPort: hubPort,
		inst: map[string]*instance{}, ports: map[string]int{}, nextPort: basePort}, nil
}

// PCM returns the channel of raw PCM (s16le, 44.1 kHz, stereo) received for a group.
// It is safe to call on a nil Manager, which means AirPlay is unavailable.
func (m *Manager) PCM(id string) (<-chan []byte, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	in, ok := m.inst[id]
	if !ok {
		return nil, false
	}
	return in.pcm, true
}

// Sync starts, restarts and stops instances so that they match groups.
func (m *Manager) Sync(groups []Group) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[string]bool{}
	for _, g := range groups {
		want[g.ID] = true
		if in, ok := m.inst[g.ID]; ok {
			if in.name == g.Name {
				continue
			}
			in.cancel()
		}
		m.start(g)
	}
	for id, in := range m.inst {
		if !want[id] {
			in.cancel()
			delete(m.inst, id)
		}
	}
}

// start must be called with m.mu held.
func (m *Manager) start(g Group) {
	port, ok := m.ports[g.ID]
	if !ok {
		port = m.nextPort
		m.nextPort++
		m.ports[g.ID] = port
	}
	ctx, cancel := context.WithCancel(m.ctx)
	in := &instance{name: g.Name, cancel: cancel, pcm: make(chan []byte, 16)}
	m.inst[g.ID] = in
	go m.run(ctx, g, port, in)
}

func (m *Manager) hook(id, action string) string {
	return fmt.Sprintf("%s notify http://127.0.0.1:%d/api/groups/%s/%s", m.hubBin, m.hubPort, id, action)
}

func (m *Manager) config(g Group, port int, fifo string) string {
	q := func(s string) string { return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) }
	return fmt.Sprintf(`general = { name = "%s"; port = %d; output_backend = "pipe"; };
pipe = { name = "%s"; };
sessioncontrol = {
	run_this_before_entering_active_state = "%s";
	run_this_after_exiting_active_state = "%s";
	active_state_timeout = %d.0;
};
`, q(g.Name), port, q(fifo), q(m.hook(g.ID, "play")), q(m.hook(g.ID, "stop")), activeTimeout)
}

func (m *Manager) run(ctx context.Context, g Group, port int, in *instance) {
	fifo := filepath.Join(m.dir, g.ID+".pcm")
	conf := filepath.Join(m.dir, g.ID+".conf")
	os.Remove(fifo)
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		log.Printf("airplay %s: mkfifo: %v", g.Name, err)
		return
	}
	defer os.Remove(fifo)
	defer os.Remove(conf)
	// Holding the FIFO open for both reading and writing keeps shairport-sync's open() from blocking.
	f, err := os.OpenFile(fifo, os.O_RDWR, 0)
	if err != nil {
		log.Printf("airplay %s: open fifo: %v", g.Name, err)
		return
	}
	go func() { <-ctx.Done(); f.Close() }()
	go func() {
		var last time.Time
		for {
			buf := make([]byte, 8192)
			n, err := f.Read(buf)
			if n > 0 {
				if time.Since(last) > 2*time.Second {
					log.Printf("airplay %s: receiving audio", g.Name)
				}
				last = time.Now()
				select {
				case in.pcm <- buf[:n]:
				default: // nobody is listening, drop it
				}
			}
			if err != nil {
				return
			}
		}
	}()
	if err := os.WriteFile(conf, []byte(m.config(g, port, fifo)), 0o644); err != nil {
		log.Printf("airplay %s: write config: %v", g.Name, err)
		return
	}
	for ctx.Err() == nil {
		log.Printf("airplay %s: starting shairport-sync on port %d", g.Name, port)
		// -u logs to stderr instead of syslog, -v adds some detail.
		cmd := exec.CommandContext(ctx, m.bin, "-u", "-v", "-c", conf)
		out, _ := cmd.StdoutPipe()
		cmd.Stderr = cmd.Stdout
		if err := cmd.Start(); err != nil {
			log.Printf("airplay %s: %v", g.Name, err)
		} else {
			sc := bufio.NewScanner(out)
			for sc.Scan() {
				log.Printf("shairport[%s]: %s", g.Name, sc.Text())
			}
			err = cmd.Wait()
			log.Printf("airplay %s: shairport-sync exited: %v", g.Name, err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
	}
}
