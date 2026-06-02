package validate

import (
	"fmt"
	"regexp"
	"strconv"
)

var (
	movieIDRe = regexp.MustCompile(`^tt\d{5,9}$`)
	actorIDRe = regexp.MustCompile(`^nm\d{5,9}$`)
)

// PageNumber parses and validates the pageNumber query parameter.
// Returns (1, nil) when s is empty (default).
// Valid range: [1, 10000].
func PageNumber(s string) (int, error) {
	if s == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 10000 {
		return 0, fmt.Errorf("pageNumber must be an integer between 1 and 10000")
	}
	return n, nil
}

// PageSize parses and validates the pageSize query parameter.
// Returns (20, nil) when s is empty (default).
// Valid range: [1, 1000].
func PageSize(s string) (int, error) {
	if s == "" {
		return 20, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 1000 {
		return 0, fmt.Errorf("pageSize must be an integer between 1 and 1000")
	}
	return n, nil
}

// Q validates a free-text search query string.
// Returns ("", nil) when s is empty (absent param — no filter applied).
// When present, valid length: [2, 20].
func Q(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	if len(s) < 2 || len(s) > 20 {
		return "", fmt.Errorf("q must be between 2 and 20 characters")
	}
	return s, nil
}

// Year parses and validates a year filter.
// Returns (0, nil) when s is empty (absent param — no filter applied).
// Valid range: [1888, 2100].
func Year(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1888 || n > 2100 {
		return 0, fmt.Errorf("year must be an integer between 1888 and 2100")
	}
	return n, nil
}

// Rating parses and validates a rating filter.
// Returns (0, nil) when s is empty (absent param — no filter applied).
// Valid range: [1.0, 10.0].
func Rating(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 1.0 || f > 10.0 {
		return 0, fmt.Errorf("rating must be a number between 1.0 and 10.0")
	}
	return f, nil
}

// ActorID validates an actorId filter parameter.
// Returns ("", nil) when s is empty (absent param — no filter applied).
// Must match ^nm\d{5,9}$.
func ActorID(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	if !actorIDRe.MatchString(s) {
		return "", fmt.Errorf("actorId must match ^nm\\d{5,9}$")
	}
	return s, nil
}

// MovieID validates a movie path parameter.
// Must match ^tt\d{5,9}$.
func MovieID(s string) (string, error) {
	if !movieIDRe.MatchString(s) {
		return "", fmt.Errorf("movie id must match ^tt\\d{5,9}$")
	}
	return s, nil
}

// ActorPathID validates an actor path parameter.
// Must match ^nm\d{5,9}$.
func ActorPathID(s string) (string, error) {
	if !actorIDRe.MatchString(s) {
		return "", fmt.Errorf("actor id must match ^nm\\d{5,9}$")
	}
	return s, nil
}
