package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/mbr/moviesx/internal/config"
	"github.com/mbr/moviesx/internal/server"
	"github.com/mbr/moviesx/internal/store"
)

var version = "0.3.0"

func main() {
	cfg := config.Load()

	var programLevel slog.LevelVar
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		lvl = slog.LevelInfo
	}
	programLevel.Set(lvl)
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: &programLevel})
	slog.SetDefault(slog.New(handler))

	slog.Info("startup", "port", cfg.Port, "log_level", cfg.LogLevel, "data_dir", cfg.DataDir)

	st, err := store.Load(cfg.DataDir)
	if err != nil {
		slog.Error("failed to load data", "err", err)
		os.Exit(1)
	}

	h := server.New(version, st)
	if err := http.ListenAndServe(":"+cfg.Port, h); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
