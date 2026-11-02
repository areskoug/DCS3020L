package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/areskoug/dcs3020l/internal/airplay"
	"github.com/areskoug/dcs3020l/internal/hub"
	"github.com/areskoug/dcs3020l/internal/web"
)

func main() {
	// "hub notify <url>" is how shairport-sync tells the running hub that playback started or ended.
	if len(os.Args) == 3 && os.Args[1] == "notify" {
		resp, err := http.Post(os.Args[2], "text/plain", nil)
		if err != nil {
			log.Fatal(err)
		}
		resp.Body.Close()
		return
	}

	// "hub volume <url> <airplay volume>" turns AirPlay's 0 .. -30 dB slider (-144 = mute) into a percentage.
	if len(os.Args) == 4 && os.Args[1] == "volume" {
		v, err := strconv.ParseFloat(os.Args[3], 64)
		if err != nil {
			log.Fatal(err)
		}
		percent := airplay.VolumePercent(v)
		resp, err := http.Post(os.Args[2], "application/json", strings.NewReader(fmt.Sprintf(`{"percent":%d}`, percent)))
		if err != nil {
			log.Fatal(err)
		}
		resp.Body.Close()
		return
	}

	port := flag.Int("port", 8080, "HTTP port for the UI, API and audio streams")
	state := flag.String("state", "groups.json", "path to the groups file (use /data/groups.json in the add-on)")
	shairport := flag.String("shairport", "shairport-sync", "shairport-sync binary used to receive AirPlay")
	airDir := flag.String("air-dir", filepath.Join(os.TempDir(), "samspel"), "directory for shairport-sync configs and pipes")
	flag.Parse()

	self, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	// HA stops the add-on with SIGTERM and expects a clean exit with code 0.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	air, err := airplay.New(ctx, *shairport, self, *airDir, *port)
	if err != nil {
		log.Printf("AirPlay disabled: %v", err)
		air = nil
	}

	h := hub.New(*port, *state, air)
	go h.Run(ctx)

	srv := &http.Server{Addr: fmt.Sprintf(":%d", *port), Handler: web.Handler(h)}
	go func() {
		<-ctx.Done()
		log.Print("shutting down")
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(shutdown) // streams are open-ended, so Shutdown gives up after the timeout
		srv.Close()
	}()
	log.Printf("listening on :%d", *port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
