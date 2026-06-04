package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	_ "github.com/mbr/moviesx/docs"
	"github.com/mbr/moviesx/internal/config"
	"github.com/mbr/moviesx/internal/server"
	"github.com/mbr/moviesx/internal/store"
)

var version = "1.0.0"

// @title          moviesx API
// @version        1.0.0
// @description    Read-only movie catalog API
// @host           localhost:8080
// @BasePath       /
// @schemes        http
func main() {
	fs := flag.NewFlagSet("moviesx", flag.ContinueOnError)

	var showVersion bool
	var port, logLevel, dataDir string

	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.BoolVar(&showVersion, "v", false, "print version and exit (shorthand)")
	fs.StringVar(&port, "movies-port", "", "listen port (env: MOVIES_PORT, default: 8080)")
	fs.StringVar(&logLevel, "movies-log-level", "", "log level (env: MOVIES_LOG_LEVEL, default: info)")
	fs.StringVar(&dataDir, "movies-data-dir", "", "data directory (env: MOVIES_DATA_DIR, default: /data)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stdout, "Usage of moviesx:\n")
		fmt.Fprintf(os.Stdout, "  --movies-port string\n\tlisten port (env: MOVIES_PORT, default: 8080)\n")
		fmt.Fprintf(os.Stdout, "  --movies-log-level string\n\tlog level (env: MOVIES_LOG_LEVEL, default: info)\n")
		fmt.Fprintf(os.Stdout, "  --movies-data-dir string\n\tdata directory (env: MOVIES_DATA_DIR, default: /data)\n")
		fmt.Fprintf(os.Stdout, "  --version, -v\n\tprint version and exit\n")
		fmt.Fprintf(os.Stdout, "  --help, -h\n\tshow this help\n")
	}

	if err := fs.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		os.Exit(2)
	}

	if showVersion {
		fmt.Fprint(os.Stdout, version)
		os.Exit(0)
	}

	cfg := config.Load(config.Flags{
		Port:     port,
		LogLevel: logLevel,
		DataDir:  dataDir,
	})

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

	slog.Info("config", "port", cfg.Port, "log_level", cfg.LogLevel, "data_dir", cfg.DataDir)

	srv, h := server.New(version, st)
	srv.SetReady()
	if err := http.ListenAndServe(":"+cfg.Port, h); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
