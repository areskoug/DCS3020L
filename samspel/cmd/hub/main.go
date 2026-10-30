package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/areskoug/dcs3020l/internal/hub"
	"github.com/areskoug/dcs3020l/internal/web"
)

func main() {
	port := flag.Int("port", 8080, "HTTP port for the UI, API and audio streams")
	state := flag.String("state", "groups.json", "path to the groups file (use /data/groups.json in the add-on)")
	flag.Parse()

	h := hub.New(*port, *state)
	log.Printf("listening on :%d", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), web.Handler(h)))
}
