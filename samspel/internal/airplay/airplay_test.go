package airplay

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func TestVolumePercent(t *testing.T) {
	for _, c := range []struct {
		in   float64
		want int
	}{{0, 100}, {-7.5, 75}, {-15, 50}, {-30, 0}, {-144, 0}, {5, 100}} {
		if got := VolumePercent(c.in); got != c.want {
			t.Errorf("VolumePercent(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestConfig(t *testing.T) {
	m := &Manager{hubBin: "/hub", hubPort: 8080}
	conf := m.config(Group{ID: "g1", Name: `Kök "stora"`}, basePort+2, "/run/g1.pcm", "/run/g1.meta")
	for _, want := range []string{
		`name = "Kök \"stora\""`,
		"port = 5102",
		"udp_port_base = 6021", // each instance gets its own block of ten ports
		`output_backend = "pipe"`,
		`ignore_volume_control = "yes"`,
		`run_this_when_volume_is_set = "/hub volume http://127.0.0.1:8080/api/groups/g1/volume "`,
		`pipe = { name = "/run/g1.pcm"; }`,
		`pipe_name = "/run/g1.meta"`,
		`run_this_before_entering_active_state = "/hub notify http://127.0.0.1:8080/api/groups/g1/play"`,
		`run_this_after_exiting_active_state = "/hub notify http://127.0.0.1:8080/api/groups/g1/stop"`,
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("config is missing %q:\n%s", want, conf)
		}
	}
}

func item(typ, code string, data string) string {
	hx := func(s string) string { return fmt.Sprintf("%x", s) }
	if data == "" {
		return fmt.Sprintf("<item><type>%s</type><code>%s</code><length>0</length></item>\n", hx(typ), hx(code))
	}
	return fmt.Sprintf("<item><type>%s</type><code>%s</code><length>%d</length>\n<data encoding=\"base64\">\n%s</data></item>\n",
		hx(typ), hx(code), len(data), base64.StdEncoding.EncodeToString([]byte(data)))
}

func TestReadMetadata(t *testing.T) {
	in := item("ssnc", "pbeg", "") +
		item("core", "minm", "I'm So Excited") +
		item("core", "asar", "The Pointer Sisters") +
		item("core", "asal", "80s 100 Hits") +
		item("ssnc", "PICT", "\xff\xd8\xffJPEGDATA")
	var st state
	readMetadata(strings.NewReader(in), &st)

	want := NowPlaying{Title: "I'm So Excited", Artist: "The Pointer Sisters", Album: "80s 100 Hits", Playing: true, CoverVer: 1}
	if st.np != want {
		t.Errorf("now playing = %+v, want %+v", st.np, want)
	}
	if string(st.cover) != "\xff\xd8\xffJPEGDATA" {
		t.Errorf("cover = %q", st.cover)
	}

	readMetadata(strings.NewReader(item("ssnc", "pend", "")), &st)
	if st.np.Playing {
		t.Error("playing should be false after pend")
	}
}

func TestReadMetadataSkipsGarbage(t *testing.T) {
	var st state
	readMetadata(strings.NewReader("noise\n"+item("core", "minm", "Ok")+"<item><type>zz"), &st)
	if st.np.Title != "Ok" {
		t.Errorf("title = %q", st.np.Title)
	}
}

func TestNilManager(t *testing.T) {
	var m *Manager
	if _, ok := m.PCM("x"); ok {
		t.Error("nil manager must report no AirPlay")
	}
	if _, ok := m.NowPlaying("x"); ok || m.Cover("x") != nil {
		t.Error("nil manager must report nothing playing")
	}
	m.Sync([]Group{{ID: "x"}}) // must not panic
}
