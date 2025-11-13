package httpapi

import "testing"

func TestFormatPoints(t *testing.T) {
    cases := []struct {
        input float64
        want  string
    }{
        {0, "0pt"},
        {30, "30pt"},
        {30.4, "30.4pt"},
        {30.49, "30.5pt"},
        {30.01, "30.0pt"},
    }
    for _, tc := range cases {
        got := formatPoints(tc.input)
        if got != tc.want {
            t.Fatalf("formatPoints(%v) = %q, want %q", tc.input, got, tc.want)
        }
    }
}

