package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/mbr/moviesx/internal/config"
	"github.com/mbr/moviesx/internal/server"
)

var version = "0.1.0"

func main() {
	cfg := config.Load()

	slog.Info("listening", "port", cfg.Port)

	mux := server.New(version)
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
