package service

import (
	"errors"
	"fmt"
)

var (
	ErrTaskNotFound   = errors.New("task not found")
	ErrTaskAmbiguous  = errors.New("task ambiguous")
	taskDefinitions   = defaultTaskDefinitions()
	taskAliasMemoizer = buildTaskAliasIndex(taskDefinitions)
)

type TaskDefinition struct {
	Key     string
	Aliases []string
	Points  float64
}

type TaskAmbiguousError struct {
	Input      string
	Candidates []string
}

func (e *TaskAmbiguousError) Error() string {
	return fmt.Sprintf("ambiguous task %q: candidates=%v", e.Input, e.Candidates)
}

func (e *TaskAmbiguousError) Unwrap() error {
	return ErrTaskAmbiguous
}

type TaskNotFoundError struct {
	Input string
}

func (e *TaskNotFoundError) Error() string {
	return fmt.Sprintf("task %q not found", e.Input)
}

func (e *TaskNotFoundError) Unwrap() error {
	return ErrTaskNotFound
}

type taskAliasIndex struct {
	exact  map[string]string
	fuzzy  map[string]string
	defMap map[string]TaskDefinition
}

func buildTaskAliasIndex(defs []TaskDefinition) taskAliasIndex {
	exact := make(map[string]string)
	fuzzy := make(map[string]string)
	defMap := make(map[string]TaskDefinition, len(defs))

	for _, def := range defs {
		defMap[normalizeCategory(def.Key)] = def
		aliases := append([]string{def.Key}, def.Aliases...)
		for _, alias := range aliases {
			normalized := normalizeCategory(alias)
			exact[normalized] = def.Key
		}
	}

	// Pre-compute fuzzy aliases for distance-1 lookups
	for alias := range exact {
		fuzzy[alias] = exact[alias]
	}

	return taskAliasIndex{
		exact:  exact,
		fuzzy:  fuzzy,
		defMap: defMap,
	}
}

func resolveTask(input string) (TaskDefinition, error) {
	normInput := normalizeCategory(input)
	if normInput == "" {
		return TaskDefinition{}, &TaskNotFoundError{Input: input}
	}

	if canonical, ok := taskAliasMemoizer.exact[normInput]; ok {
		return taskAliasMemoizer.defMap[normalizeCategory(canonical)], nil
	}

	candidates := make(map[string]struct{})
	for alias, canonical := range taskAliasMemoizer.fuzzy {
		if levenshteinDistance(alias, normInput) <= 1 {
			candidates[canonical] = struct{}{}
		}
	}

	switch len(candidates) {
	case 0:
		return TaskDefinition{}, &TaskNotFoundError{Input: input}
	case 1:
		for canonical := range candidates {
			return taskAliasMemoizer.defMap[normalizeCategory(canonical)], nil
		}
	default:
		names := make([]string, 0, len(candidates))
		for canonical := range candidates {
			names = append(names, canonical)
		}
		return TaskDefinition{}, &TaskAmbiguousError{
			Input:      input,
			Candidates: names,
		}
	}
	return TaskDefinition{}, &TaskNotFoundError{Input: input}
}

func levenshteinDistance(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}

	previous := make([]int, len(br)+1)
	current := make([]int, len(br)+1)

	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(ar); i++ {
		current[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			current[j] = min(
				current[j-1]+1,
				previous[j]+1,
				previous[j-1]+cost,
			)
		}
		copy(previous, current)
	}

	return previous[len(br)]
}

func min(vals ...int) int {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

const BASE_POINT = 100

func defaultTaskDefinitions() []TaskDefinition {
	return []TaskDefinition{
		{
			Key:     "皿洗い",
			Aliases: []string{"さらあらい", "皿洗い", "洗い物", "洗いもの"},
			Points:  BASE_POINT * 3,
		},
		{
			Key:     "洗濯",
			Aliases: []string{"せんたく", "洗濯", "洗濯物", "せんたくもの", "せんたく物"},
			Points:  BASE_POINT,
		},
		{
			Key:     "ゴミ出し",
			Aliases: []string{"ごみだし", "ゴミ出し", "ゴミ", "ごみ"},
			Points:  BASE_POINT * 1.5,
		},
		{
			Key:     "買い出し",
			Aliases: []string{"買出し", "買い出し", "買い物", "買いもの", "かいもの"},
			Points:  BASE_POINT * 4,
		},
		{
			Key:     "風呂掃除",
			Aliases: []string{"ふろそうじ", "風呂掃除", "風呂清掃", "風呂", "ふろ"},
			Points:  BASE_POINT * 1.5,
		},
		{
			Key:     "トイレ掃除",
			Aliases: []string{"トイレそうじ", "トイレ掃除", "トイレ清掃", "トイレ", "といれそうじ", "といれ"},
			Points:  BASE_POINT * 4,
		},
		{
			Key:     "床掃除",
			Aliases: []string{"ゆかそうじ", "床掃除", "床清掃", "床", "ゆか"},
			Points:  BASE_POINT * 3,
		},
		{
			Key:     "洗面台掃除",
			Aliases: []string{"洗面台掃除", "洗面台清掃", "せんめんだい", "せんめんだいそうじ", "洗面台"},
			Points:  BASE_POINT * 3,
		},
		{
			Key:     "風呂排水溝",
			Aliases: []string{"風呂の排水溝", "排水溝風呂", "風呂排水溝", "風呂の排水溝"},
			Points:  BASE_POINT * 3,
		},
	}
}
