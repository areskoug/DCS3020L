package airplay

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// NowPlaying is what the AirPlay source says it is playing.
type NowPlaying struct {
	Title    string `json:"title,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	Playing  bool   `json:"playing"`
	CoverVer int    `json:"coverVer,omitempty"` // changes whenever new cover art arrives
}

// state is the metadata of one instance, filled in by readMetadata.
type state struct {
	mu    sync.Mutex
	np    NowPlaying
	cover []byte
}

var itemHeader = regexp.MustCompile(`<item><type>([0-9a-f]{8})</type><code>([0-9a-f]{8})</code><length>(\d+)</length>`)

// readMetadata parses shairport-sync's metadata pipe: items of the form
//
//	<item><type>636f7265</type><code>6d696e6d</code><length>5</length>
//	<data encoding="base64">
//	SGVsbG8=</data></item>
//
// where type and code are four ASCII characters in hex. It returns when r ends.
func readMetadata(r io.Reader, st *state) {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadString('\n')
		if m := itemHeader.FindStringSubmatch(line); m != nil {
			typ, code := hexString(m[1]), hexString(m[2])
			var data []byte
			if n, _ := strconv.Atoi(m[3]); n > 0 {
				data = readData(br)
			}
			st.apply(typ, code, data)
		}
		if err != nil {
			return
		}
	}
}

// readData reads the base64 block that follows an item header, up to </data>.
func readData(br *bufio.Reader) []byte {
	var sb strings.Builder
	for {
		line, err := br.ReadString('\n')
		sb.WriteString(line)
		if i := strings.Index(sb.String(), "</data>"); i >= 0 || err != nil {
			s := sb.String()
			if i >= 0 {
				s = s[:i]
			}
			s = strings.TrimPrefix(strings.TrimSpace(s), `<data encoding="base64">`)
			b, _ := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(s), ""))
			return b
		}
	}
}

func hexString(h string) string {
	b, _ := hex.DecodeString(h)
	return string(b)
}

func (st *state) apply(typ, code string, data []byte) {
	st.mu.Lock()
	defer st.mu.Unlock()
	switch typ + "/" + code {
	case "core/minm":
		st.np.Title = string(data)
	case "core/asar":
		st.np.Artist = string(data)
	case "core/asal":
		st.np.Album = string(data)
	case "ssnc/PICT":
		st.cover = data
		st.np.CoverVer++
	case "ssnc/pbeg", "ssnc/prsm":
		st.np.Playing = true
	case "ssnc/pend", "ssnc/pfls":
		st.np.Playing = false
	}
}

// VolumePercent turns AirPlay's volume (0 down to -30 dB, -144 meaning mute) into a percentage.
func VolumePercent(airplay float64) int {
	p := int((30+airplay)/30*100 + 0.5)
	return max(0, min(100, p))
}
