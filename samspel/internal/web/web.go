// Package web serves the control UI, the JSON API and the live audio streams.
package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/areskoug/dcs3020l/internal/devices"
	"github.com/areskoug/dcs3020l/internal/hub"
	"github.com/areskoug/dcs3020l/internal/stream"
)

//go:embed static
var static embed.FS

func Handler(h *hub.Hub) http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(static, "static")
	mux.Handle("/", http.FileServer(http.FS(sub)))

	mux.HandleFunc("GET /api/devices", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, h.Devices()) })
	mux.HandleFunc("POST /api/discover", func(w http.ResponseWriter, r *http.Request) {
		err := h.Discover(r.Context())
		resp := map[string]any{"devices": h.Devices()}
		if err != nil {
			resp["error"] = err.Error()
		}
		writeJSON(w, resp)
	})
	mux.HandleFunc("GET /api/groups", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, h.Groups()) })
	mux.HandleFunc("POST /api/groups", func(w http.ResponseWriter, r *http.Request) {
		var g hub.Group
		if json.NewDecoder(r.Body).Decode(&g) != nil || strings.TrimSpace(g.Name) == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		writeJSON(w, h.SaveGroup(g))
	})
	mux.HandleFunc("DELETE /api/groups/{id}", func(w http.ResponseWriter, r *http.Request) {
		h.DeleteGroup(r.Context(), r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/groups/{id}/play", func(w http.ResponseWriter, r *http.Request) {
		if err := h.StartGroup(r.Context(), r.PathValue("id"), r.URL.Query().Get("tone") == "1"); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/groups/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
		h.StopGroup(r.Context(), r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/groups/{id}/cover", func(w http.ResponseWriter, r *http.Request) {
		img := h.Cover(r.PathValue("id"))
		if len(img) == 0 {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", http.DetectContentType(img))
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(img)
	})
	mux.HandleFunc("POST /api/groups/{id}/volume", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Percent int `json:"percent"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		h.SetGroupVolume(r.PathValue("id"), clamp(body.Percent, 0, 100))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/devices/{id}/format", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Format string `json:"format"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || !h.SetDeviceFormat(r.PathValue("id"), body.Format) {
			http.Error(w, "unknown device or format", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/devices/{id}/volume", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Percent int `json:"percent"`
		}
		d, ok := h.Device(r.PathValue("id"))
		if !ok || json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := devices.NewPlayer(d).SetVolume(r.Context(), body.Percent); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// The delay lives in the path because HEOS passes a percent-encoded '=' on to the stream request unchanged.
	mux.HandleFunc("GET /stream/{id}/{file}", func(w http.ResponseWriter, r *http.Request) {
		file := r.PathValue("file")
		stem, ext, _ := strings.Cut(file, ".")
		serveStream(h, w, r, r.PathValue("id"), time.Duration(clamp(atoi(stem), 0, 40000))*time.Millisecond, stream.FormatByExt(ext))
	})
	mux.HandleFunc("GET /stream/{name}", func(w http.ResponseWriter, r *http.Request) {
		serveStream(h, w, r, strings.TrimSuffix(r.PathValue("name"), ".mp3"), 0, stream.Formats[0])
	})
	return mux
}

// cushion is the audio a speaker gets up front, in addition to its delay. Without it a speaker starts with an
// empty buffer, runs dry at once because the live audio only arrives in real time, stalls and rebuffers.
const cushion = 3 * time.Second

func serveStream(h *hub.Hub, w http.ResponseWriter, r *http.Request, id string, delay time.Duration, f stream.Format) {
	header, ch, unsub, ok := h.Subscribe(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	defer unsub()
	lead, err := stream.Silence(f, delay+cushion)
	if err != nil {
		log.Printf("stream %s: %v", id, err)
		http.Error(w, "no audio", http.StatusInternalServerError)
		return
	}
	log.Printf("stream %s: %s connected, %v of silence in front (%s)", id, r.RemoteAddr, delay+cushion, r.UserAgent())
	defer log.Printf("stream %s: %s disconnected", id, r.RemoteAddr)
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Cache-Control", "no-cache")
	fl, _ := w.(http.Flusher)
	write := func(b []byte) bool {
		if _, err := w.Write(b); err != nil {
			return false
		}
		if fl != nil {
			fl.Flush()
		}
		return true
	}
	if !write(header) || !write(lead) {
		return
	}
	for {
		select {
		case chunk := <-ch:
			if !write(chunk) {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func clamp(n, lo, hi int) int { return max(lo, min(hi, n)) }
