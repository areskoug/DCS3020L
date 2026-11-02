package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

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

	port := flag.Int("port", 8080, "HTTP port for the UI, API and audio streams")
	state := flag.String("state", "groups.json", "path to the groups file (use /data/groups.json in the add-on)")
	shairport := flag.String("shairport", "shairport-sync", "shairport-sync binary used to receive AirPlay")
	airDir := flag.String("air-dir", filepath.Join(os.TempDir(), "samspel"), "directory for shairport-sync configs and pipes")
	flag.Parse()

	self, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	air, err := airplay.New(context.Background(), *shairport, self, *airDir, *port)
	if err != nil {
		log.Printf("AirPlay disabled: %v", err)
		air = nil
	}

	h := hub.New(*port, *state, air)
	log.Printf("listening on :%d", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), web.Handler(h)))
}
