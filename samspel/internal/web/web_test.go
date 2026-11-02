package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/areskoug/dcs3020l/internal/hub"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "g.devices.json"),
		[]byte(`[{"id":"cast-2","name":"Vardagsrum","kind":"cast","addr":"10.0.0.2","port":8009}]`), 0o644)
	srv := httptest.NewServer(Handler(hub.New(8080, filepath.Join(dir, "g.json"), nil)))
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestGroupAPI(t *testing.T) {
	srv := testServer(t)

	if r := do(t, "POST", srv.URL+"/api/groups", `{"name":"  "}`); r.StatusCode != 400 {
		t.Errorf("blank name: %d", r.StatusCode)
	}
	var g hub.Group
	r := do(t, "POST", srv.URL+"/api/groups", `{"name":"Uppe","deviceIds":["cast-2"]}`)
	if json.NewDecoder(r.Body).Decode(&g); g.ID == "" || g.Name != "Uppe" {
		t.Fatalf("created %+v", g)
	}

	var all []hub.Group
	json.NewDecoder(do(t, "GET", srv.URL+"/api/groups", "").Body).Decode(&all)
	if len(all) != 2 { // the new group and the solo group for the device
		t.Errorf("got %d groups", len(all))
	}

	if r := do(t, "DELETE", srv.URL+"/api/groups/"+g.ID, ""); r.StatusCode != 204 {
		t.Errorf("delete: %d", r.StatusCode)
	}
}

func TestInactiveStreamAndCoverAreNotFound(t *testing.T) {
	srv := testServer(t)
	for _, path := range []string{"/stream/nope.mp3", "/stream/nope/0.mp3", "/api/groups/nope/cover"} {
		if r := do(t, "GET", srv.URL+path, ""); r.StatusCode != 404 {
			t.Errorf("%s: %d", path, r.StatusCode)
		}
	}
}

func TestVolumeRejectsBadBody(t *testing.T) {
	srv := testServer(t)
	if r := do(t, "POST", srv.URL+"/api/groups/x/volume", "not json"); r.StatusCode != 400 {
		t.Errorf("status %d", r.StatusCode)
	}
	if r := do(t, "POST", srv.URL+"/api/groups/x/volume", `{"percent":50}`); r.StatusCode != 204 {
		t.Errorf("status %d", r.StatusCode)
	}
}

func TestClamp(t *testing.T) {
	if clamp(-5, 0, 10) != 0 || clamp(50, 0, 10) != 10 || clamp(5, 0, 10) != 5 || atoi("x") != 0 || atoi("42") != 42 {
		t.Error("clamp/atoi")
	}
}

func TestIndexIsServed(t *testing.T) {
	srv := testServer(t)
	r := do(t, "GET", srv.URL+"/", "")
	if r.StatusCode != 200 {
		t.Errorf("status %d", r.StatusCode)
	}
}

func TestDeviceFormatAPI(t *testing.T) {
	srv := testServer(t)
	if r := do(t, "POST", srv.URL+"/api/devices/cast-2/format", `{"format":"wav"}`); r.StatusCode != 204 {
		t.Errorf("wav: %d", r.StatusCode)
	}
	if r := do(t, "POST", srv.URL+"/api/devices/cast-2/format", `{"format":"ogg"}`); r.StatusCode != 400 {
		t.Errorf("ogg: %d", r.StatusCode)
	}
	if r := do(t, "POST", srv.URL+"/api/devices/nope/format", `{"format":"wav"}`); r.StatusCode != 400 {
		t.Errorf("unknown device: %d", r.StatusCode)
	}
	var devs []map[string]any
	json.NewDecoder(do(t, "GET", srv.URL+"/api/devices", "").Body).Decode(&devs)
	if len(devs) != 1 || devs[0]["format"] != "wav" {
		t.Errorf("devices = %v", devs)
	}
}
