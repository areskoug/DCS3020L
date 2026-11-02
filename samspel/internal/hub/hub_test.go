package hub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/areskoug/dcs3020l/internal/devices"
)

func newTestHub(t *testing.T, groups, devs string) (*Hub, string) {
	t.Helper()
	dir := t.TempDir()
	state := filepath.Join(dir, "groups.json")
	if groups != "" {
		os.WriteFile(state, []byte(groups), 0o644)
	}
	if devs != "" {
		os.WriteFile(filepath.Join(dir, "groups.devices.json"), []byte(devs), 0o644)
	}
	return New(8080, state, nil), state
}

const twoDevices = `[{"id":"heos-1","name":"Kök","kind":"heos","addr":"10.0.0.1","port":1255,"heosPid":"1"},
{"id":"cast-2","name":"Vardagsrum","kind":"cast","addr":"10.0.0.2","port":8009}]`

func byID(h *Hub) map[string]Group {
	m := map[string]Group{}
	for _, g := range h.Groups() {
		m[g.ID] = g
	}
	return m
}

func TestEveryDeviceGetsASoloGroup(t *testing.T) {
	h, _ := newTestHub(t, "", twoDevices)
	gs := byID(h)
	if g := gs["solo-cast-2"]; !g.Solo || g.Name != "Vardagsrum" || len(g.DeviceIDs) != 1 || g.DeviceIDs[0] != "cast-2" {
		t.Errorf("unexpected solo group %+v", g)
	}
	if len(gs) != 2 {
		t.Errorf("got %d groups, want 2", len(gs))
	}
}

func TestSoloNameAvoidsClashWithUserGroup(t *testing.T) {
	h, _ := newTestHub(t, `[{"id":"g1","name":"Kök","deviceIds":["heos-1"]}]`, twoDevices)
	gs := byID(h)
	if gs["g1"].Name != "Kök" || gs["solo-heos-1"].Name != "Kök (högtalare)" {
		t.Errorf("names: %q and %q", gs["g1"].Name, gs["solo-heos-1"].Name)
	}
}

func TestSoloGroupsAreNeverStoredOrChanged(t *testing.T) {
	h, state := newTestHub(t, "", twoDevices)
	h.SaveGroup(Group{Name: "Uppe", DeviceIDs: []string{"cast-2"}})
	h.DeleteGroup(t.Context(), "solo-cast-2")
	h.SaveGroup(Group{ID: "solo-cast-2", Name: "Hijacked"})

	if g := byID(h)["solo-cast-2"]; g.Name != "Vardagsrum" {
		t.Errorf("solo group was changed: %+v", g)
	}
	data, _ := os.ReadFile(state)
	if strings.Contains(string(data), "solo-") || !strings.Contains(string(data), "Uppe") {
		t.Errorf("stored groups should hold only user groups:\n%s", data)
	}
}

func TestGroupSettingsSurviveARestart(t *testing.T) {
	h, state := newTestHub(t, "", twoDevices)
	h.SaveGroup(Group{ID: "g1", Name: "Uppe", DeviceIDs: []string{"heos-1", "cast-2"},
		DelaysMs: map[string]int{"heos-1": 16250}, TrimsPct: map[string]int{"heos-1": 60}})

	h2 := New(8080, state, nil)
	g := byID(h2)["g1"]
	if g.DelaysMs["heos-1"] != 16250 || g.TrimsPct["heos-1"] != 60 {
		t.Errorf("settings lost: %+v", g)
	}
	if _, ok := h2.Device("cast-2"); !ok {
		t.Error("devices should be remembered across restarts")
	}
}

func TestStreamURLCarriesDelayInThePath(t *testing.T) {
	h, _ := newTestHub(t, "", "")
	url, err := h.streamURL(devices.Device{Addr: "127.0.0.1"}, "g1", 16250, "mp3")
	if err != nil || !strings.HasSuffix(url, ":8080/stream/g1/16250.mp3") || strings.ContainsAny(url[7:], "?&=") {
		t.Errorf("url = %q, err = %v", url, err)
	}
}

func TestStartingUnknownGroupFails(t *testing.T) {
	h, _ := newTestHub(t, "", "")
	ctx := t.Context()
	if err := h.StartGroup(ctx, "nope", false); err == nil {
		t.Error("expected an error")
	}
}

func TestDelayKeyKeepsFormatsApart(t *testing.T) {
	if DelayKey("heos-1", "") != "heos-1" || DelayKey("heos-1", "mp3-192") != "heos-1" {
		t.Error("the default format must use the plain device ID")
	}
	if DelayKey("heos-1", "wav") != "heos-1@wav" {
		t.Error("other formats must get their own key")
	}
}

func TestDeviceFormatIsKeptAcrossRestartAndShownOnTheSoloGroup(t *testing.T) {
	h, state := newTestHub(t, "", twoDevices)
	if !h.SetDeviceFormat("cast-2", "wav") {
		t.Fatal("wav should be accepted")
	}
	if g := byID(h)["solo-cast-2"]; g.Format != "wav" {
		t.Errorf("solo format = %q", g.Format)
	}
	if byID(h)["solo-heos-1"].Format != "" {
		t.Error("other speakers must keep the default")
	}

	h2 := New(8080, state, nil)
	if byID(h2)["solo-cast-2"].Format != "wav" {
		t.Error("format lost after restart")
	}
}

func TestDeviceFormatValidation(t *testing.T) {
	h, _ := newTestHub(t, "", twoDevices)
	if h.SetDeviceFormat("cast-2", "ogg") || h.SetDeviceFormat("nope", "wav") {
		t.Error("unknown format or device must be refused")
	}
	h.SetDeviceFormat("cast-2", "wav")
	h.SetDeviceFormat("cast-2", "mp3-192") // naming the default clears the setting
	if d, _ := h.Device("cast-2"); d.Format != "" {
		t.Errorf("format = %q, want default", d.Format)
	}
}

func TestCushionDefault(t *testing.T) {
	for _, kind := range []devices.Kind{devices.Cast, devices.HEOS} {
		if got := (devices.Device{Kind: kind}).Cushion(); got != 5 {
			t.Errorf("%s default cushion = %d, want 5", kind, got)
		}
	}
	if (devices.Device{Kind: devices.Cast, CushionSec: 8}).Cushion() != 8 {
		t.Error("an explicit cushion must win")
	}
}

func TestCushion(t *testing.T) {
	h, state := newTestHub(t, "", twoDevices)
	if !h.SetDeviceCushion("cast-2", 8) {
		t.Fatal("8 s should be accepted")
	}
	if h.SetDeviceCushion("cast-2", 0) || h.SetDeviceCushion("cast-2", 31) || h.SetDeviceCushion("nope", 5) {
		t.Error("out of range values and unknown devices must be refused")
	}
	if d, _ := New(8080, state, nil).Device("cast-2"); d.Cushion() != 8 {
		t.Errorf("cushion lost after restart: %d", d.Cushion())
	}
	h.SetDeviceCushion("cast-2", 5) // naming the default clears the setting
	if d, _ := h.Device("cast-2"); d.CushionSec != 0 {
		t.Errorf("CushionSec = %d, want unset", d.CushionSec)
	}
}

func TestDevicesListShowsTheCushionInEffect(t *testing.T) {
	h, _ := newTestHub(t, "", twoDevices)
	got := map[string]int{}
	for _, d := range h.Devices() {
		got[d.ID] = d.CushionSec
	}
	if got["cast-2"] != 5 || got["heos-1"] != 5 {
		t.Errorf("cushions shown: %v", got)
	}
	if d, _ := h.Device("cast-2"); d.CushionSec != 0 {
		t.Error("listing must not store the default")
	}
}
