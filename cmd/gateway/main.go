package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/server"
	"github.com/sunqirui1987/xhub/internal/store"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "LiteLLM-shaped YAML config")
	addr := flag.String("addr", ":4000", "listen address")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	srv := server.New(cfg, st)
	log.Printf("xhub listening on %s", *addr)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
