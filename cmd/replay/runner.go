package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Config holds all CLI-derived settings passed to runner functions.
type Config struct {
	BaseURL     string
	Scenarios   string
	Benchmark   bool
	Duration    time.Duration
	Concurrency int
	RPS         float64
	Timeout     time.Duration
	Verbose     bool
}

// Result is the outcome of a single HTTP request execution.
type Result struct {
	ScenarioID string
	Duration   time.Duration
	StatusCode int
	Body       []byte
	Headers    http.Header
	Err        error
}

// DiscoverIDs fetches a valid movie ID and actor ID from the live server.
func DiscoverIDs(baseURL string, client *http.Client) (movieID, actorID string, err error) {
	movieID, err = discoverFirstID(client, baseURL+"/api/movies?pageSize=1", "movie")
	if err != nil {
		return "", "", err
	}
	actorID, err = discoverFirstID(client, baseURL+"/api/actors?pageSize=1", "actor")
	if err != nil {
		return "", "", err
	}
	return movieID, actorID, nil
}

func discoverFirstID(client *http.Client, u, kind string) (string, error) {
	resp, err := client.Get(u)
	if err != nil {
		return "", fmt.Errorf("discover %s ID: %w", kind, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("discover %s ID: unexpected status %d", kind, resp.StatusCode)
	}
	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return "", fmt.Errorf("discover %s ID: decode: %w", kind, err)
	}
	if len(page.Items) == 0 {
		return "", fmt.Errorf("discover %s ID: empty items array", kind)
	}
	return page.Items[0].ID, nil
}

// applyTemplates substitutes {known_movie_id} and {known_actor_id} in the scenario path.
func applyTemplates(s Scenario, movieID, actorID string) Scenario {
	s.Path = strings.ReplaceAll(s.Path, "{known_movie_id}", movieID)
	s.Path = strings.ReplaceAll(s.Path, "{known_actor_id}", actorID)
	return s
}

// Execute issues one HTTP request and returns the result with measured duration.
// Duration is measured around client.Do only, not assertion.
func Execute(ctx context.Context, client *http.Client, baseURL string, s Scenario) Result {
	u := baseURL + s.Path
	if len(s.Query) > 0 {
		q := url.Values{}
		for k, v := range s.Query {
			q.Set(k, v)
		}
		u += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, s.Method, u, nil)
	if err != nil {
		return Result{ScenarioID: s.ID, Err: err}
	}

	start := time.Now()
	resp, err := client.Do(req)
	dur := time.Since(start)

	if err != nil {
		return Result{ScenarioID: s.ID, Duration: dur, Err: err}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return Result{
		ScenarioID: s.ID,
		Duration:   dur,
		StatusCode: resp.StatusCode,
		Body:       body,
		Headers:    resp.Header,
	}
}

// RunBaseline runs all scenarios sequentially and prints per-scenario pass/fail lines.
// Returns (pass, fail, err). Callers should exit 1 when fail > 0.
func RunBaseline(cfg Config, scenarios []Scenario) (pass, fail int, err error) {
	client := noRedirectClient(cfg.Timeout)

	movieID, actorID, err := DiscoverIDs(cfg.BaseURL, client)
	if err != nil {
		return 0, 0, fmt.Errorf("ID discovery: %w", err)
	}

	for _, s := range scenarios {
		s = applyTemplates(s, movieID, actorID)
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		r := Execute(ctx, client, cfg.BaseURL, s)
		cancel()

		if r.Err != nil {
			fail++
			fmt.Printf("FAIL  %-35s ERR   %-6s %-40s %s\n",
				s.ID, s.Method, s.Path, r.Err)
			continue
		}

		errs := Assert(s, r)
		if len(errs) == 0 {
			pass++
			if cfg.Verbose {
				fmt.Printf("PASS  %-35s %3d   %-6s %-40s %s\n",
					s.ID, r.StatusCode, s.Method, s.Path, fmtDur(r.Duration))
			}
		} else {
			fail++
			fmt.Printf("FAIL  %-35s %3d   %-6s %-40s %s\n",
				s.ID, r.StatusCode, s.Method, s.Path, fmtDur(r.Duration))
			for _, e := range errs {
				fmt.Printf("      ↳ %s\n", e)
			}
		}
	}

	fmt.Printf("---\nRan %d scenarios: %d passed, %d failed\n", len(scenarios), pass, fail)
	return pass, fail, nil
}

// benchResult is a lightweight result struct used by the benchmark collector.
type benchResult struct {
	scenarioID string
	duration   time.Duration
	status     int
	err        error
}

// RunBenchmark runs a sustained load test and prints a latency summary table.
// Returns non-nil error only on startup failure; exit code is controlled by target checks.
func RunBenchmark(cfg Config, scenarios []Scenario) error {
	if len(scenarios) == 0 {
		return fmt.Errorf("no scenarios loaded")
	}

	client := noRedirectClient(cfg.Timeout)

	movieID, actorID, err := DiscoverIDs(cfg.BaseURL, client)
	if err != nil {
		return fmt.Errorf("ID discovery: %w", err)
	}

	applied := make([]Scenario, len(scenarios))
	for i, s := range scenarios {
		applied[i] = applyTemplates(s, movieID, actorID)
	}

	cap := int(cfg.RPS*cfg.Duration.Seconds()*1.2) + 100
	jobs := make(chan struct{}, 2*cfg.Concurrency)
	resultsCh := make(chan benchResult, 2*cfg.Concurrency)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
	defer cancel()

	// Rate-limiter: fires one job token per tick.
	tickInterval := time.Duration(float64(time.Second) / cfg.RPS)
	ticker := time.NewTicker(tickInterval)
	go func() {
		defer close(jobs)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				select {
				case jobs <- struct{}{}:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Workers: round-robin over scenarios.
	var wg sync.WaitGroup
	var counter int64
	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				idx := int(atomic.AddInt64(&counter, 1)-1) % len(applied)
				s := applied[idx]
				reqCtx, reqCancel := context.WithTimeout(ctx, cfg.Timeout)
				r := Execute(reqCtx, client, cfg.BaseURL, s)
				reqCancel()
				resultsCh <- benchResult{
					scenarioID: s.ID,
					duration:   r.Duration,
					status:     r.StatusCode,
					err:        r.Err,
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collector: accumulate per-scenario durations.
	type scenarioStats struct {
		label     string
		durations []time.Duration
		errors    int
		total     int
	}
	statsMap := make(map[string]*scenarioStats, len(applied))
	for _, s := range applied {
		statsMap[s.ID] = &scenarioStats{
			label:     s.Method + " " + s.Path,
			durations: make([]time.Duration, 0, cap/len(applied)),
		}
	}
	var totalRequests, totalErrors int64

	for r := range resultsCh {
		atomic.AddInt64(&totalRequests, 1)
		st := statsMap[r.scenarioID]
		st.total++
		if r.err != nil || r.status == 0 || r.status >= 500 {
			atomic.AddInt64(&totalErrors, 1)
			st.errors++
		} else {
			st.durations = append(st.durations, r.duration)
		}
	}

	// Sort durations for percentile calculation.
	for _, st := range statsMap {
		sort.Slice(st.durations, func(i, j int) bool { return st.durations[i] < st.durations[j] })
	}

	// Print table.
	fmt.Printf("%-35s %8s %7s %8s %8s %8s\n", "endpoint", "requests", "errors", "p50", "p95", "p99")
	for _, s := range applied {
		st := statsMap[s.ID]
		p50 := percentile(st.durations, 0.50)
		p95 := percentile(st.durations, 0.95)
		p99 := percentile(st.durations, 0.99)
		fmt.Printf("%-35s %8d %7d %8s %8s %8s\n",
			st.label, st.total, st.errors,
			fmtDur(p50), fmtDur(p95), fmtDur(p99))
	}
	fmt.Println()

	// Check targets per spec §10.4.
	allPass := true
	for _, s := range applied {
		st := statsMap[s.ID]
		p95 := percentile(st.durations, 0.95)
		target, ok := p95Target(s)
		if !ok {
			continue
		}
		label := st.label
		if p95 <= target {
			fmt.Printf("PASS: p95 %s = %s < %s target\n", label, fmtDur(p95), fmtDur(target))
		} else {
			fmt.Printf("FAIL: p95 %s = %s >= %s target\n", label, fmtDur(p95), fmtDur(target))
			allPass = false
		}
	}

	errRate := 0.0
	if totalRequests > 0 {
		errRate = float64(totalErrors) / float64(totalRequests) * 100
	}
	if errRate < 1.0 {
		fmt.Printf("PASS: error rate = %.2f%% < 1%% target\n", errRate)
	} else {
		fmt.Printf("FAIL: error rate = %.2f%% >= 1%% target\n", errRate)
		allPass = false
	}

	if !allPass {
		return fmt.Errorf("one or more benchmark targets missed")
	}
	return nil
}

// p95Target returns the p95 latency target for a benchmark scenario based on its path.
func p95Target(s Scenario) (time.Duration, bool) {
	switch {
	case s.Method == "GET" && s.Path == "/api/movies":
		return 50 * time.Millisecond, true
	case s.Method == "GET" && strings.HasPrefix(s.Path, "/api/movies/"):
		return 10 * time.Millisecond, true
	default:
		return 0, false
	}
}

// percentile returns the p-th percentile of a pre-sorted duration slice.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func fmtDur(d time.Duration) string {
	return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
}

// noRedirectClient creates an http.Client that does not follow redirects
// and uses a tuned transport for high-concurrency benchmark scenarios.
func noRedirectClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 200,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
