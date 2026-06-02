package validate

import "testing"

func TestPageNumber(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"", 1, false},        // default
		{"1", 1, false},       // lower boundary
		{"0", 0, true},        // lower boundary - 1
		{"10000", 10000, false}, // upper boundary
		{"10001", 0, true},    // upper boundary + 1
		{"abc", 0, true},      // non-numeric
		{"-1", 0, true},       // negative
		{"99", 99, false},     // valid mid-range
	}
	for _, tc := range cases {
		got, err := PageNumber(tc.in)
		if tc.wantErr && err == nil {
			t.Errorf("PageNumber(%q): want error, got nil (value=%d)", tc.in, got)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("PageNumber(%q): unexpected error: %v", tc.in, err)
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("PageNumber(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestPageSize(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"", 20, false},      // default
		{"1", 1, false},      // lower boundary
		{"0", 0, true},       // lower boundary - 1
		{"1000", 1000, false}, // upper boundary
		{"1001", 0, true},    // upper boundary + 1
		{"abc", 0, true},     // non-numeric
		{"20", 20, false},    // valid
	}
	for _, tc := range cases {
		got, err := PageSize(tc.in)
		if tc.wantErr && err == nil {
			t.Errorf("PageSize(%q): want error, got nil (value=%d)", tc.in, got)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("PageSize(%q): unexpected error: %v", tc.in, err)
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("PageSize(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestQ(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},                      // absent param
		{"ab", "ab", false},                  // lower boundary (2 chars)
		{"a", "", true},                      // lower boundary - 1 (1 char)
		{"12345678901234567890", "12345678901234567890", false}, // upper boundary (20 chars)
		{"123456789012345678901", "", true},   // upper boundary + 1 (21 chars)
		{"hello", "hello", false},            // valid mid-range
	}
	for _, tc := range cases {
		got, err := Q(tc.in)
		if tc.wantErr && err == nil {
			t.Errorf("Q(%q): want error, got nil (value=%q)", tc.in, got)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("Q(%q): unexpected error: %v", tc.in, err)
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("Q(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestYear(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"", 0, false},      // absent param
		{"1888", 1888, false}, // lower boundary
		{"1887", 0, true},   // lower boundary - 1
		{"2100", 2100, false}, // upper boundary
		{"2101", 0, true},   // upper boundary + 1
		{"abc", 0, true},    // non-numeric
		{"2001", 2001, false}, // valid
	}
	for _, tc := range cases {
		got, err := Year(tc.in)
		if tc.wantErr && err == nil {
			t.Errorf("Year(%q): want error, got nil (value=%d)", tc.in, got)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("Year(%q): unexpected error: %v", tc.in, err)
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("Year(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestRating(t *testing.T) {
	cases := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{"", 0, false},       // absent param
		{"1.0", 1.0, false},  // lower boundary
		{"0.9", 0, true},     // lower boundary - 0.1
		{"10.0", 10.0, false}, // upper boundary
		{"10.1", 0, true},    // upper boundary + 0.1
		{"abc", 0, true},     // non-numeric
		{"8.5", 8.5, false},  // valid
		{"0", 0, true},       // zero — outside domain
		{"11", 0, true},      // well above max
	}
	for _, tc := range cases {
		got, err := Rating(tc.in)
		if tc.wantErr && err == nil {
			t.Errorf("Rating(%q): want error, got nil (value=%v)", tc.in, got)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("Rating(%q): unexpected error: %v", tc.in, err)
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("Rating(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestActorID(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},                // absent param
		{"nm0000001", "nm0000001", false}, // valid 7-digit
		{"nm00001", "nm00001", false},  // valid 5-digit (lower boundary)
		{"nm000000001", "nm000000001", false}, // valid 9-digit (upper boundary)
		{"nm0000", "", true},           // 4 digits — too short
		{"nm0000000001", "", true},     // 10 digits — too long
		{"tt0000001", "", true},        // wrong prefix
		{"nm", "", true},               // no digits
		{"abcdefgh", "", true},         // no prefix
	}
	for _, tc := range cases {
		got, err := ActorID(tc.in)
		if tc.wantErr && err == nil {
			t.Errorf("ActorID(%q): want error, got nil (value=%q)", tc.in, got)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("ActorID(%q): unexpected error: %v", tc.in, err)
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("ActorID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMovieID(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"tt0000001", "tt0000001", false}, // valid 7-digit
		{"tt00001", "tt00001", false},     // valid 5-digit (lower boundary)
		{"tt000000001", "tt000000001", false}, // valid 9-digit (upper boundary)
		{"tt0000", "", true},              // 4 digits — too short
		{"tt0000000001", "", true},        // 10 digits — too long
		{"nm0000001", "", true},           // wrong prefix
		{"tt", "", true},                  // no digits
		{"", "", true},                    // empty string
		{"abcdefgh", "", true},            // no prefix
	}
	for _, tc := range cases {
		got, err := MovieID(tc.in)
		if tc.wantErr && err == nil {
			t.Errorf("MovieID(%q): want error, got nil (value=%q)", tc.in, got)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("MovieID(%q): unexpected error: %v", tc.in, err)
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("MovieID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestActorPathID(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"nm0000001", false},
		{"nm00001", false},
		{"nm000000001", false},
		{"nm0000", true},
		{"tt0000001", true},
		{"", true},
	}
	for _, tc := range cases {
		_, err := ActorPathID(tc.in)
		if tc.wantErr && err == nil {
			t.Errorf("ActorPathID(%q): want error, got nil", tc.in)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("ActorPathID(%q): unexpected error: %v", tc.in, err)
		}
	}
}
