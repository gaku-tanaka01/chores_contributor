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

	wt := 1.0
	w, _ := s.rp.CategoryWeight(ctx, p.GroupID, canonical)
	if w > 0 {
		wt = w
	}
	points := def.Points * wt

	return s.rp.InsertEvent(ctx, repo.InsertEventParams{
		ExtGroupID:  p.GroupID,
		ExtUserID:   p.UserID,
		Category:    &canonical,
		Points:      points,
		SourceMsgID: p.SourceMsgID,
		Now:         now,
		Note:        p.Note,
	})
}

func (s *Service) Rp() *repo.Repo {
	return s.rp
}

func (s *Service) UpsertCategory(ctx context.Context, group, name string, weight float64) error {
	normalized := normalizeCategory(name)
	return s.rp.UpsertCategory(ctx, group, normalized, weight)
}
