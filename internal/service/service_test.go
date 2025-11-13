package service

import (
	"errors"
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

func withTaskDefinitions(defs []TaskDefinition, fn func()) {
    oldDefs := taskDefinitions
    oldMemo := taskAliasMemoizer
    taskDefinitions = defs
    taskAliasMemoizer = buildTaskAliasIndex(defs)
    defer func() {
        taskDefinitions = oldDefs
        taskAliasMemoizer = oldMemo
    }()
    fn()
}

func TestResolveTaskExact(t *testing.T) {
    withTaskDefinitions([]TaskDefinition{{Key: "皿洗い", Aliases: []string{"さらあらい"}, Points: 123}}, func() {
        def, err := resolveTask("皿洗い")
        if err != nil {
            t.Fatalf("resolveTask returned error: %v", err)
        }
        if def.Key != "皿洗い" || def.Points != 123 {
            t.Fatalf("unexpected definition: %+v", def)
        }
    })
}

func TestResolveTaskFuzzy(t *testing.T) {
    withTaskDefinitions([]TaskDefinition{{Key: "皿洗い", Aliases: []string{"さらあらい"}, Points: 50}}, func() {
        def, err := resolveTask("皿洗")
        if err != nil {
            t.Fatalf("resolveTask returned error: %v", err)
        }
        if def.Key != "皿洗い" {
            t.Fatalf("expected 皿洗い, got %s", def.Key)
        }
    })
}

func TestResolveTaskAmbiguous(t *testing.T) {
    withTaskDefinitions([]TaskDefinition{
        {Key: "aaaa", Points: 10},
        {Key: "aaab", Points: 8},
    }, func() {
        _, err := resolveTask("aaaf")
        if err == nil {
            t.Fatalf("expected ambiguity error")
        }
        var amb *TaskAmbiguousError
        if !errors.As(err, &amb) {
            t.Fatalf("expected TaskAmbiguousError, got %v", err)
        }
        if len(amb.Candidates) != 2 {
            t.Fatalf("expected 2 candidates, got %v", amb.Candidates)
        }
    })
}

func TestResolveTaskNotFound(t *testing.T) {
    withTaskDefinitions([]TaskDefinition{{Key: "皿洗い", Aliases: []string{"さらあらい"}, Points: 10}}, func() {
        _, err := resolveTask("掃除")
        if !errors.Is(err, ErrTaskNotFound) {
            t.Fatalf("expected ErrTaskNotFound, got %v", err)
        }
    })
}

