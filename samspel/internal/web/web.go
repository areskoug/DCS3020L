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
		if err := h.StartGroup(r.Context(), r.PathValue("id")); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/groups/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
		h.StopGroup(r.Context(), r.PathValue("id"))
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
		ms := atoi(strings.TrimSuffix(r.PathValue("file"), ".mp3"))
		serveStream(h, w, r, r.PathValue("id"), time.Duration(clamp(ms, 0, 40000))*time.Millisecond)
	})
	mux.HandleFunc("GET /stream/{name}", func(w http.ResponseWriter, r *http.Request) {
		serveStream(h, w, r, strings.TrimSuffix(r.PathValue("name"), ".mp3"), 0)
	})
	return mux
}

func serveStream(h *hub.Hub, w http.ResponseWriter, r *http.Request, id string, delay time.Duration) {
	ch, unsub, ok := h.Subscribe(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	defer unsub()
	log.Printf("stream %s: %s connected, delay %v (%s)", id, r.RemoteAddr, delay, r.UserAgent())
	defer log.Printf("stream %s: %s disconnected", id, r.RemoteAddr)
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-cache")
	fl, _ := w.(http.Flusher)
	for {
		select {
		case chunk := <-ch:
			if wait := time.Until(chunk.At.Add(delay)); wait > 0 {
				select {
				case <-time.After(wait):
				case <-r.Context().Done():
					return
				}
			}
			if _, err := w.Write(chunk.Data); err != nil {
				return
			}
			if fl != nil {
				fl.Flush()
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
