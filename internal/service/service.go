package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"chores_contributor/internal/repo"

	"golang.org/x/text/unicode/norm"
)

var jst = must(time.LoadLocation("Asia/Tokyo"))

func must(loc *time.Location, err error) *time.Location {
	if err != nil {
		panic(err)
	}
	return loc
}

func nowJST() time.Time {
	return time.Now().In(jst)
}

type Service struct{ rp *repo.Repo }

func New(rp *repo.Repo) *Service { return &Service{rp: rp} }

// normalizeCategory カテゴリ名を正規化（全角/半角・NFKC・trim・連続空白圧縮）
func normalizeCategory(s string) string {
	s = strings.TrimSpace(s)
	// 連続空白を単一スペースに圧縮
	s = strings.Join(strings.Fields(s), " ")
	// Unicode正規化（NFKC: 互換等価文字を統合）
	s = norm.NFKC.String(s)
	return s
}

type ReportPayload struct {
	GroupID     string  `json:"group_id"` // ext_group_id
	UserID      string  `json:"user_id"`  // ext_user_id
	Task        string  `json:"task"`
	Option      *string `json:"option,omitempty"`
	Type        *string `json:"type,omitempty"`
	SourceMsgID *string `json:"source_msg_id,omitempty"`
	Note        *string `json:"note,omitempty"`
}

type WeeklyTaskSummary struct {
	TaskKey string
	Points  float64
}

type WeeklyUserSummary struct {
	Total    float64
	TaskList []WeeklyTaskSummary
}

type CancelResult struct {
	TaskKey string
	Points  float64
}

func (s *Service) Report(ctx context.Context, p ReportPayload) error {
	if p.GroupID == "" || p.UserID == "" {
		return errors.New("missing required fields")
	}
	if p.SourceMsgID == nil || *p.SourceMsgID == "" {
		return errors.New("source_msg_id is required for idempotency")
	}
	if p.Type != nil && *p.Type != "" && *p.Type != string(repo.KindChore) {
		return errors.New("type must be 'chore' when provided")
	}
	if strings.TrimSpace(p.Task) == "" {
		return errors.New("task is required")
	}
	now := nowJST()

	def, err := resolveTask(strings.TrimSpace(p.Task))
	if err != nil {
		return err
	}
	canonical := normalizeCategory(def.Key)
	points := def.Points

	return s.rp.InsertEvent(ctx, repo.InsertEventParams{
		ExtGroupID:  p.GroupID,
		ExtUserID:   p.UserID,
		TaskKey:     canonical,
		TaskOption:  p.Option,
		Points:      points,
		SourceMsgID: p.SourceMsgID,
		Now:         now,
		Note:        p.Note,
	})
}

func (s *Service) Rp() *repo.Repo {
	return s.rp
}

func (s *Service) WeeklyUserSummary(ctx context.Context, groupID, userID string, ref time.Time) (WeeklyUserSummary, error) {
	wd := int(ref.Weekday())
	if wd == 0 {
		wd = 7
	}
	start := time.Date(ref.Year(), ref.Month(), ref.Day(), 0, 0, 0, 0, ref.Location()).AddDate(0, 0, -(wd - 1))
	end := start.AddDate(0, 0, 7)

	rows, err := s.rp.WeeklyUserTaskPoints(ctx, groupID, userID, start, end)
	if err != nil {
		return WeeklyUserSummary{}, err
	}

	summary := WeeklyUserSummary{TaskList: make([]WeeklyTaskSummary, 0, len(rows))}
	for _, row := range rows {
		summary.Total += row.Points
		summary.TaskList = append(summary.TaskList, WeeklyTaskSummary{
			TaskKey: row.TaskKey,
			Points:  row.Points,
		})
	}
	return summary, nil
}

func (s *Service) CancelLatestEvent(ctx context.Context, groupID, userID string) (CancelResult, error) {
	deleted, err := s.rp.DeleteLatestEvent(ctx, groupID, userID)
	if err != nil {
		return CancelResult{}, err
	}
	return CancelResult{TaskKey: deleted.TaskKey, Points: deleted.Points}, nil
}

func (s *Service) TaskDefinitions() []TaskDefinition {
	out := make([]TaskDefinition, len(taskDefinitions))
	copy(out, taskDefinitions)
	return out
}
