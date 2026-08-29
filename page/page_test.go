package page

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type testCursor struct {
	Name string `json:"n"`
	Pos  int    `json:"p"`
}

func TestEncodeDecode_RoundTrip(t *testing.T) {
	want := testCursor{Name: "Album", Pos: 3}

	token, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode(%+v) failed: %v", want, err)
	}

	var got testCursor
	if err := Decode(token, &got); err != nil {
		t.Fatalf("Decode(%q) failed: %v", token, err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Decode(Encode(%+v)) mismatch (-want +got):\n%s", want, diff)
	}
}

func TestEncode_IsURLSafe(t *testing.T) {
	// Tokens travel in query params, so the encoding must not emit
	// characters that need escaping.
	token, err := Encode(testCursor{Name: "A/B+C?D=E"})
	if err != nil {
		t.Fatalf("Encode() failed: %v", err)
	}
	for _, r := range token {
		if !isURLSafe(r) {
			t.Errorf("Encode() = %q, want only unreserved characters, got %q", token, r)
		}
	}
}

func isURLSafe(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '-', r == '_':
		return true
	}
	return false
}

func TestDecode_Malformed(t *testing.T) {
	var got testCursor
	err := Decode("not-a-token!", &got)
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf(`Decode("not-a-token!") error = %v, want ErrInvalidToken`, err)
	}
}

func TestDecode_ForeignCursor(t *testing.T) {
	// A token minted for one listing must not decode into another
	// listing's cursor, silently zeroing the fields it lacks.
	type otherCursor struct {
		Other string `json:"o"`
	}
	token, err := Encode(testCursor{Name: "Album", Pos: 3})
	if err != nil {
		t.Fatalf("Encode() failed: %v", err)
	}

	var got otherCursor
	if err := Decode(token, &got); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Decode(token from another listing) error = %v, want ErrInvalidToken", err)
	}
}

func TestSize(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want int
	}{
		{name: "zero", n: 0, want: DefaultSize},
		{name: "negative", n: -1, want: DefaultSize},
		{name: "within range", n: 10, want: 10},
		{name: "over max", n: MaxSize + 1, want: MaxSize},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Size(tc.n); got != tc.want {
				t.Errorf("Size(%d) = %d, want %d", tc.n, got, tc.want)
			}
		})
	}
}
