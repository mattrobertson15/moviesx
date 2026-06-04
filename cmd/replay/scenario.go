package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// AssertBlock holds all body assertion options for a scenario.
type AssertBlock struct {
	HasKeys            []string       `yaml:"has_keys"`
	IsArray            bool           `yaml:"is_array"`
	ArrayMinLength     int            `yaml:"array_min_length"`
	BodyContains       map[string]any `yaml:"body_contains"`
	BodyMinItems       *int           `yaml:"body_min_items"`
	BodyContainsString string         `yaml:"body_contains_string"`
	BodyMatchesRegex   string         `yaml:"body_matches_regex"`
}

// Scenario is one test case: a request definition plus expected response.
type Scenario struct {
	ID            string            `yaml:"id"`
	Method        string            `yaml:"method"`
	Path          string            `yaml:"path"`
	Query         map[string]string `yaml:"query"`
	ExpectStatus  int               `yaml:"expect_status"`
	ExpectHeaders map[string]string `yaml:"expect_headers"`
	Assert        AssertBlock       `yaml:"assert"`
}

// ScenariosFile is the top-level structure of a scenario YAML file.
type ScenariosFile struct {
	Scenarios []Scenario `yaml:"scenarios"`
}

// LoadScenarios reads and merges scenario files, returning an error on duplicate IDs.
func LoadScenarios(paths []string) ([]Scenario, error) {
	var all []Scenario
	seen := map[string]bool{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", p, err)
		}
		var sf ScenariosFile
		if err := yaml.Unmarshal(data, &sf); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", p, err)
		}
		for _, s := range sf.Scenarios {
			if seen[s.ID] {
				return nil, fmt.Errorf("duplicate scenario ID: %s", s.ID)
			}
			seen[s.ID] = true
			all = append(all, s)
		}
	}
	return all, nil
}

// ExpandGlob wraps filepath.Glob and errors if the pattern matches zero files.
func ExpandGlob(pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob %q: %w", pattern, err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no files matched pattern %q", pattern)
	}
	return matches, nil
}
