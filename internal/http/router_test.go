package httpapi

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

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

func TestFetchLineDisplayName(t *testing.T) {
	ctx := context.Background()
	oldClient := http.DefaultClient
	defer func() { http.DefaultClient = oldClient }()
	oldToken := os.Getenv("LINE_CHANNEL_ACCESS_TOKEN")
	if err := os.Setenv("LINE_CHANNEL_ACCESS_TOKEN", "test-token"); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	defer func() { _ = os.Setenv("LINE_CHANNEL_ACCESS_TOKEN", oldToken) }()

	type request struct {
		method string
		host   string
		path   string
	}

	tests := []struct {
		name        string
		src         lineSource
		expectPath  string
		response    string
		wantName    *string
		expectError bool
	}{
		{
			name:       "group member profile",
			src:        lineSource{GroupID: "g123", UserID: "Uabc"},
			expectPath: "/v2/bot/group/g123/member/Uabc",
			response:   `{"displayName":"Alice"}`,
			wantName:   ptr("Alice"),
		},
		{
			name:       "room member profile",
			src:        lineSource{RoomID: "r555", UserID: "Uxyz"},
			expectPath: "/v2/bot/room/r555/member/Uxyz",
			response:   `{"displayName":"Bob"}`,
			wantName:   ptr("Bob"),
		},
		{
			name:       "direct profile blank name",
			src:        lineSource{UserID: "U000"},
			expectPath: "/v2/bot/profile/U000",
			response:   `{"displayName":"   "}`,
			wantName:   nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var got request
			http.DefaultClient = &http.Client{
				Transport: roundTripper(func(req *http.Request) (*http.Response, error) {
					got = request{method: req.Method, host: req.URL.Host, path: req.URL.Path}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(tt.response)),
						Header:     make(http.Header),
					}, nil
				}),
			}

			name, err := fetchLineDisplayName(ctx, tt.src)
			if err != nil {
				t.Fatalf("fetchLineDisplayName returned error: %v", err)
			}
			if got.method != http.MethodGet {
				t.Fatalf("expected GET request, got %s", got.method)
			}
			if got.host != "api.line.me" {
				t.Fatalf("expected host api.line.me, got %s", got.host)
			}
			if got.path != tt.expectPath {
				t.Fatalf("expected path %s, got %s", tt.expectPath, got.path)
			}
			if tt.wantName == nil {
				if name != nil {
					t.Fatalf("expected nil name, got %q", *name)
				}
			} else if name == nil || *name != *tt.wantName {
				if name == nil {
					t.Fatalf("expected name %q, got nil", *tt.wantName)
				}
				t.Fatalf("expected name %q, got %q", *tt.wantName, *name)
			}
		})
	}
}

func TestFetchLineDisplayNameError(t *testing.T) {
	ctx := context.Background()
	oldClient := http.DefaultClient
	defer func() { http.DefaultClient = oldClient }()
	oldToken := os.Getenv("LINE_CHANNEL_ACCESS_TOKEN")
	if err := os.Setenv("LINE_CHANNEL_ACCESS_TOKEN", "test-token"); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	defer func() { _ = os.Setenv("LINE_CHANNEL_ACCESS_TOKEN", oldToken) }()

	http.DefaultClient = &http.Client{
		Transport: roundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader(`{"message":"not found"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	src := lineSource{GroupID: "g", UserID: "u"}
	name, err := fetchLineDisplayName(ctx, src)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if name != nil {
		t.Fatalf("expected nil name on error, got %q", *name)
	}
}

func ptr(s string) *string { return &s }

type roundTripper func(*http.Request) (*http.Response, error)

func (rt roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}
