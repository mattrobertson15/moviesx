package main

import (
	"net/http"
	"testing"
)

func makeResult(status int, body string) Result {
	return Result{
		StatusCode: status,
		Body:       []byte(body),
		Headers:    make(http.Header),
	}
}

func TestAssert_StatusPass(t *testing.T) {
	s := Scenario{ExpectStatus: 200}
	r := makeResult(200, `{"items":[]}`)
	errs := Assert(s, r)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestAssert_StatusFail(t *testing.T) {
	s := Scenario{ExpectStatus: 200}
	r := makeResult(400, `{"error":"bad"}`)
	errs := Assert(s, r)
	if len(errs) != 1 || errs[0].Field != "status" {
		t.Fatalf("expected status error, got %v", errs)
	}
}

func TestAssert_StatusFailEarlyReturn(t *testing.T) {
	// body assertions are skipped when status mismatches
	s := Scenario{
		ExpectStatus: 200,
		Assert:       AssertBlock{HasKeys: []string{"items"}},
	}
	r := makeResult(400, `{"error":"bad"}`)
	errs := Assert(s, r)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error (status only), got %v", errs)
	}
}

func TestAssert_HasKeysMissing(t *testing.T) {
	s := Scenario{
		ExpectStatus: 200,
		Assert:       AssertBlock{HasKeys: []string{"items", "total", "missing"}},
	}
	r := makeResult(200, `{"items":[],"total":0}`)
	errs := Assert(s, r)
	if len(errs) != 1 || errs[0].Expected != "key missing" {
		t.Fatalf("expected missing-key error, got %v", errs)
	}
}

func TestAssert_HasKeysPresent(t *testing.T) {
	s := Scenario{
		ExpectStatus: 200,
		Assert:       AssertBlock{HasKeys: []string{"items", "total"}},
	}
	r := makeResult(200, `{"items":[],"total":0,"page":1,"pageSize":20}`)
	errs := Assert(s, r)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestAssert_BodyContainsWrongValue(t *testing.T) {
	s := Scenario{
		ExpectStatus: 200,
		Assert:       AssertBlock{BodyContains: map[string]any{"total": 1}},
	}
	r := makeResult(200, `{"total":0}`)
	errs := Assert(s, r)
	if len(errs) != 1 || errs[0].Field != "body_contains.total" {
		t.Fatalf("expected body_contains mismatch error, got %v", errs)
	}
}

func TestAssert_BodyContainsCorrectValue(t *testing.T) {
	s := Scenario{
		ExpectStatus: 200,
		Assert:       AssertBlock{BodyContains: map[string]any{"page": 1, "pageSize": 5}},
	}
	r := makeResult(200, `{"page":1,"pageSize":5}`)
	errs := Assert(s, r)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestAssert_IsArrayOnObjectFails(t *testing.T) {
	s := Scenario{
		ExpectStatus: 200,
		Assert:       AssertBlock{IsArray: true},
	}
	r := makeResult(200, `{"foo":1}`)
	errs := Assert(s, r)
	if len(errs) != 1 || errs[0].Field != "is_array" {
		t.Fatalf("expected is_array error, got %v", errs)
	}
}

func TestAssert_IsArrayPasses(t *testing.T) {
	s := Scenario{
		ExpectStatus: 200,
		Assert:       AssertBlock{IsArray: true, ArrayMinLength: 2},
	}
	r := makeResult(200, `["a","b","c"]`)
	errs := Assert(s, r)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestAssert_IsArrayMinLengthFails(t *testing.T) {
	s := Scenario{
		ExpectStatus: 200,
		Assert:       AssertBlock{IsArray: true, ArrayMinLength: 5},
	}
	r := makeResult(200, `["a","b"]`)
	errs := Assert(s, r)
	if len(errs) != 1 || errs[0].Field != "array_min_length" {
		t.Fatalf("expected array_min_length error, got %v", errs)
	}
}

func TestAssert_RegexPass(t *testing.T) {
	s := Scenario{
		ExpectStatus: 200,
		Assert:       AssertBlock{BodyMatchesRegex: `^\d+\.\d+\.\d+$`},
	}
	r := makeResult(200, "1.2.3")
	errs := Assert(s, r)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestAssert_RegexMismatchFails(t *testing.T) {
	s := Scenario{
		ExpectStatus: 200,
		Assert:       AssertBlock{BodyMatchesRegex: `^\d+\.\d+\.\d+$`},
	}
	r := makeResult(200, "1.0") // only two segments — does not match semver pattern
	errs := Assert(s, r)
	if len(errs) != 1 || errs[0].Field != "body_matches_regex" {
		t.Fatalf("expected regex mismatch error, got %v", errs)
	}
}

func TestAssert_BodyContainsString(t *testing.T) {
	s := Scenario{
		ExpectStatus: 200,
		Assert:       AssertBlock{BodyContainsString: "http_requests_total"},
	}
	r := makeResult(200, "# HELP http_requests_total total requests\nhttp_requests_total{} 5")
	errs := Assert(s, r)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestAssert_BodyContainsStringMissing(t *testing.T) {
	s := Scenario{
		ExpectStatus: 200,
		Assert:       AssertBlock{BodyContainsString: "not_present"},
	}
	r := makeResult(200, "other content")
	errs := Assert(s, r)
	if len(errs) != 1 || errs[0].Field != "body_contains_string" {
		t.Fatalf("expected body_contains_string error, got %v", errs)
	}
}

func TestAssert_BodyMinItems(t *testing.T) {
	minItems := 1
	s := Scenario{
		ExpectStatus: 200,
		Assert:       AssertBlock{BodyMinItems: &minItems},
	}
	r := makeResult(200, `{"items":[{"id":"tt001"}],"total":1}`)
	errs := Assert(s, r)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestAssert_BodyMinItemsFails(t *testing.T) {
	minItems := 3
	s := Scenario{
		ExpectStatus: 200,
		Assert:       AssertBlock{BodyMinItems: &minItems},
	}
	r := makeResult(200, `{"items":[],"total":0}`)
	errs := Assert(s, r)
	if len(errs) != 1 || errs[0].Field != "body_min_items" {
		t.Fatalf("expected body_min_items error, got %v", errs)
	}
}

func TestWalkPath_Simple(t *testing.T) {
	obj := map[string]any{"total": float64(42)}
	v, err := walkPath(obj, "total")
	if err != nil || v.(float64) != 42 {
		t.Fatalf("expected 42, got %v, err=%v", v, err)
	}
}

func TestWalkPath_Nested(t *testing.T) {
	obj := map[string]any{
		"items": []any{
			map[string]any{"id": "tt001"},
		},
	}
	v, err := walkPath(obj, "items.0.id")
	if err != nil || v.(string) != "tt001" {
		t.Fatalf("expected tt001, got %v, err=%v", v, err)
	}
}
