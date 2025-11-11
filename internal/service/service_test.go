package service

import (
	"testing"
)

func TestNormalizeCategory(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"trim spaces", "  皿洗い  ", "皿洗い"},
		{"multiple spaces", "皿洗い  掃除", "皿洗い 掃除"},
		{"empty", "", ""},
		{"normal", "皿洗い", "皿洗い"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeCategory(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeCategory(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

