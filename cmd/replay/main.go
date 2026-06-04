package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	var cfg Config
	flag.StringVar(&cfg.BaseURL, "base-url", "", "base URL of target service, e.g. http://10.42.0.5:8080 (required)")
	flag.StringVar(&cfg.Scenarios, "scenarios", "", "scenario file path or glob pattern (required)")
	flag.BoolVar(&cfg.Benchmark, "benchmark", false, "enable sustained-load benchmark mode instead of functional suite")
	flag.DurationVar(&cfg.Duration, "duration", 30*time.Second, "benchmark run duration")
	flag.IntVar(&cfg.Concurrency, "concurrency", 10, "concurrent workers in benchmark mode")
	flag.Float64Var(&cfg.RPS, "rps", 500, "target requests per second in benchmark mode")
	flag.DurationVar(&cfg.Timeout, "timeout", 5*time.Second, "per-request HTTP client timeout")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "print all scenarios (pass and fail) in baseline mode")
	flag.Parse()

	if cfg.BaseURL == "" {
		fmt.Fprintln(os.Stderr, "error: --base-url is required")
		flag.Usage()
		os.Exit(1)
	}
	if cfg.Scenarios == "" {
		fmt.Fprintln(os.Stderr, "error: --scenarios is required")
		flag.Usage()
		os.Exit(1)
	}

	paths, err := ExpandGlob(cfg.Scenarios)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	scenarios, err := LoadScenarios(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if cfg.Benchmark {
		if err := RunBenchmark(cfg, scenarios); err != nil {
			fmt.Fprintln(os.Stderr, "benchmark:", err)
			os.Exit(1)
		}
	} else {
		_, fail, err := RunBaseline(cfg, scenarios)
		if err != nil {
			fmt.Fprintln(os.Stderr, "baseline:", err)
			os.Exit(1)
		}
		if fail > 0 {
			os.Exit(1)
		}
	}
}
