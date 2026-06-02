package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/mbr/moviesx/internal/config"
	"github.com/mbr/moviesx/internal/server"
	"github.com/mbr/moviesx/internal/store"
)

var version = "0.2.0"

func main() {
	cfg := config.Load()

	st, err := store.Load(cfg.DataDir)
	if err != nil {
		slog.Error("failed to load data", "err", err)
		os.Exit(1)
	}

	slog.Info("listening", "port", cfg.Port)

	mux := server.New(version, st)
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
