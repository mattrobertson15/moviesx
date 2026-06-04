package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// AssertionError describes a single failed assertion.
type AssertionError struct {
	Field    string
	Expected string
	Got      string
}

func (e AssertionError) String() string {
	return fmt.Sprintf("%s: expected %s, got %s", e.Field, e.Expected, e.Got)
}

// Assert runs all assertions for a scenario against a result.
// Status mismatch causes early return — body assertions are meaningless on wrong-status responses.
func Assert(s Scenario, r Result) []AssertionError {
	var errs []AssertionError

	// 1. Status code
	if r.StatusCode != s.ExpectStatus {
		return []AssertionError{{
			Field:    "status",
			Expected: fmt.Sprintf("%d", s.ExpectStatus),
			Got:      fmt.Sprintf("%d", r.StatusCode),
		}}
	}

	// 2. Expected headers (substring match, case-insensitive)
	for key, substr := range s.ExpectHeaders {
		got := r.Headers.Get(key)
		if !strings.Contains(strings.ToLower(got), strings.ToLower(substr)) {
			errs = append(errs, AssertionError{
				Field:    "header:" + key,
				Expected: "contains " + substr,
				Got:      got,
			})
		}
	}

	body := string(r.Body)

	// 3. body_contains_string
	if s.Assert.BodyContainsString != "" {
		if !strings.Contains(body, s.Assert.BodyContainsString) {
			errs = append(errs, AssertionError{
				Field:    "body_contains_string",
				Expected: s.Assert.BodyContainsString,
				Got:      "(not found)",
			})
		}
	}

	// 4. body_matches_regex (trimmed to avoid trailing newline issues)
	if s.Assert.BodyMatchesRegex != "" {
		re, err := regexp.Compile(s.Assert.BodyMatchesRegex)
		if err != nil {
			errs = append(errs, AssertionError{
				Field:    "body_matches_regex",
				Expected: "valid regex",
				Got:      "compile error: " + err.Error(),
			})
		} else if !re.MatchString(strings.TrimSpace(body)) {
			errs = append(errs, AssertionError{
				Field:    "body_matches_regex",
				Expected: s.Assert.BodyMatchesRegex,
				Got:      strings.TrimSpace(body),
			})
		}
	}

	// 5. is_array
	if s.Assert.IsArray {
		var arr []any
		if err := json.Unmarshal(r.Body, &arr); err != nil {
			errs = append(errs, AssertionError{
				Field:    "is_array",
				Expected: "JSON array",
				Got:      "parse error: " + err.Error(),
			})
		} else if s.Assert.ArrayMinLength > 0 && len(arr) < s.Assert.ArrayMinLength {
			errs = append(errs, AssertionError{
				Field:    "array_min_length",
				Expected: fmt.Sprintf(">= %d", s.Assert.ArrayMinLength),
				Got:      fmt.Sprintf("%d", len(arr)),
			})
		}
	}

	// 6. has_keys
	if len(s.Assert.HasKeys) > 0 {
		var obj map[string]any
		if err := json.Unmarshal(r.Body, &obj); err != nil {
			errs = append(errs, AssertionError{
				Field:    "has_keys",
				Expected: "JSON object",
				Got:      "parse error: " + err.Error(),
			})
		} else {
			for _, k := range s.Assert.HasKeys {
				if _, ok := obj[k]; !ok {
					errs = append(errs, AssertionError{
						Field:    "has_keys",
						Expected: "key " + k,
						Got:      "(missing)",
					})
				}
			}
		}
	}

	// 7. body_contains — dot-path subset equality
	if len(s.Assert.BodyContains) > 0 {
		var obj map[string]any
		if err := json.Unmarshal(r.Body, &obj); err != nil {
			errs = append(errs, AssertionError{
				Field:    "body_contains",
				Expected: "JSON object",
				Got:      "parse error: " + err.Error(),
			})
		} else {
			for k, want := range s.Assert.BodyContains {
				got, err := walkPath(obj, k)
				if err != nil {
					errs = append(errs, AssertionError{
						Field:    "body_contains." + k,
						Expected: fmt.Sprintf("%v", want),
						Got:      err.Error(),
					})
					continue
				}
				if !valuesEqual(got, want) {
					errs = append(errs, AssertionError{
						Field:    "body_contains." + k,
						Expected: fmt.Sprintf("%v", want),
						Got:      fmt.Sprintf("%v", got),
					})
				}
			}
		}
	}

	// 8. body_min_items — shorthand for items array length in Page envelope
	if s.Assert.BodyMinItems != nil {
		var obj map[string]any
		if err := json.Unmarshal(r.Body, &obj); err != nil {
			errs = append(errs, AssertionError{
				Field:    "body_min_items",
				Expected: "JSON object",
				Got:      "parse error: " + err.Error(),
			})
		} else {
			items, ok := obj["items"].([]any)
			if !ok {
				errs = append(errs, AssertionError{
					Field:    "body_min_items",
					Expected: "items array",
					Got:      "(not an array or missing)",
				})
			} else if len(items) < *s.Assert.BodyMinItems {
				errs = append(errs, AssertionError{
					Field:    "body_min_items",
					Expected: fmt.Sprintf(">= %d", *s.Assert.BodyMinItems),
					Got:      fmt.Sprintf("%d", len(items)),
				})
			}
		}
	}

	return errs
}

// walkPath follows a dot-separated key path through nested JSON structures.
// Supports map keys and integer array indices: "items.0.id".
func walkPath(root map[string]any, path string) (any, error) {
	parts := strings.Split(path, ".")
	var cur any = root
	for i, part := range parts {
		switch v := cur.(type) {
		case map[string]any:
			val, ok := v[part]
			if !ok {
				return nil, fmt.Errorf("key %q not found", part)
			}
			cur = val
		case []any:
			idx, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("expected array index at segment %d, got %q", i, part)
			}
			if idx < 0 || idx >= len(v) {
				return nil, fmt.Errorf("index %d out of range (len=%d)", idx, len(v))
			}
			cur = v[idx]
		default:
			return nil, fmt.Errorf("cannot traverse into %T at segment %d (%q)", cur, i, part)
		}
	}
	return cur, nil
}

// valuesEqual compares JSON-decoded values against YAML-decoded expected values.
// JSON numbers are float64; YAML integers are int — handles the conversion.
func valuesEqual(got, want any) bool {
	if gf, ok := got.(float64); ok {
		switch w := want.(type) {
		case int:
			return gf == float64(w)
		case float64:
			return gf == w
		}
	}
	return fmt.Sprintf("%v", got) == fmt.Sprintf("%v", want)
}
