package main

import (
	"fmt"
	"log"
	"net/http"

	"quotio-lite/internal/config"
	"quotio-lite/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	app := server.New(cfg)
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("quotio-lite listening on http://%s", addr)
	if err := http.ListenAndServe(addr, app.Handler()); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
