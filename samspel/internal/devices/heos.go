package devices

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

const (
	heosPort = 1255
	// heosSearchTarget is the SSDP search target from the HEOS CLI spec.
	heosSearchTarget = "urn:schemas-denon-com:device:ACT-Denon:1"
)

// DiscoverHEOS finds HEOS speakers via SSDP, then asks one of them for the full player list.
func DiscoverHEOS(ctx context.Context, d time.Duration) ([]Device, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	msg := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: " + heosSearchTarget + "\r\n\r\n"
	dst := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}
	if _, err := conn.WriteTo([]byte(msg), dst); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(d)
	conn.SetReadDeadline(deadline)
	var first string
	buf := make([]byte, 2048)
	for first == "" {
		_, from, err := conn.ReadFrom(buf)
		if err != nil || ctx.Err() != nil {
			break
		}
		first = from.(*net.UDPAddr).IP.String()
	}
	if first == "" {
		return nil, nil
	}
	// Any HEOS device reports every player in the network.
	resp, err := heosCommand(first, "heos://player/get_players", "player/get_players")
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Payload []struct {
			Name string `json:"name"`
			Pid  int    `json:"pid"`
			IP   string `json:"ip"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return nil, err
	}
	var out []Device
	for _, p := range parsed.Payload {
		out = append(out, Device{ID: fmt.Sprintf("heos-%d", p.Pid), Name: p.Name, Kind: HEOS, Addr: p.IP, Port: heosPort, HEOSPid: fmt.Sprint(p.Pid)})
	}
	return out, nil
}

// heosCommand sends one CLI command and returns the response line whose "command" field
// equals wantCommand. Unsolicited event lines are skipped.
func heosCommand(addr, cmd, wantCommand string) ([]byte, error) {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(addr, fmt.Sprint(heosPort)), 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	log.Printf("heos %s <- %s", addr, cmd)
	if _, err := fmt.Fprintf(conn, "heos://system/register_for_change_events?enable=off\r\n%s\r\n", cmd); err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var head struct {
			Heos struct {
				Command string `json:"command"`
				Message string `json:"message"`
				Result  string `json:"result"`
			} `json:"heos"`
		}
		if json.Unmarshal(sc.Bytes(), &head) != nil || head.Heos.Command != wantCommand {
			continue
		}
		log.Printf("heos %s -> %s", addr, sc.Bytes())
		if head.Heos.Result == "fail" {
			return nil, fmt.Errorf("heos %s failed: %s", wantCommand, head.Heos.Message)
		}
		return append([]byte(nil), sc.Bytes()...), nil
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("heos: no response to %s", wantCommand)
}

// heosEscape encodes only the characters the HEOS CLI spec reserves ('&', '=' and '%').
func heosEscape(s string) string {
	return strings.NewReplacer("%", "%25", "&", "%26", "=", "%3D").Replace(s)
}

type heosPlayer struct{ dev Device }

func (p *heosPlayer) Play(_ context.Context, streamURL string) error {
	cmd := fmt.Sprintf("heos://browse/play_stream?pid=%s&url=%s", p.dev.HEOSPid, heosEscape(streamURL))
	_, err := heosCommand(p.dev.Addr, cmd, "browse/play_stream")
	return err
}

func (p *heosPlayer) Stop(_ context.Context) error {
	_, err := heosCommand(p.dev.Addr, fmt.Sprintf("heos://player/set_play_state?pid=%s&state=stop", p.dev.HEOSPid), "player/set_play_state")
	return err
}

func (p *heosPlayer) SetVolume(_ context.Context, percent int) error {
	level := strings.TrimSpace(fmt.Sprint(max(0, min(100, percent))))
	_, err := heosCommand(p.dev.Addr, fmt.Sprintf("heos://player/set_volume?pid=%s&level=%s", p.dev.HEOSPid, level), "player/set_volume")
	return err
}
