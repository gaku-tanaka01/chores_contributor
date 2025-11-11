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
	GroupID     string  `json:"group_id"`          // ext_group_id
	UserID      string  `json:"user_id"`           // ext_user_id
	Type        string  `json:"type"`              // "chore" or "purchase"
	Category    *string `json:"category,omitempty"`
	Minutes     *int    `json:"minutes,omitempty"` // when chore
	AmountYen   *int    `json:"amount_yen,omitempty"`
	SourceMsgID *string `json:"source_msg_id,omitempty"`
	Note        *string `json:"note,omitempty"`
}

func (s *Service) Report(ctx context.Context, p ReportPayload) error {
	if p.GroupID == "" || p.UserID == "" || p.Type == "" {
		return errors.New("missing required fields")
	}
	if p.SourceMsgID == nil || *p.SourceMsgID == "" {
		return errors.New("source_msg_id is required for idempotency")
	}
	now := nowJST()

	var points float64
	switch p.Type {
	case string(repo.KindChore):
		if p.Minutes == nil || *p.Minutes <= 0 {
			return errors.New("minutes required (>0)")
		}
		wt := 1.0
		if p.Category != nil {
			normalized := normalizeCategory(*p.Category)
			p.Category = &normalized
			w, _ := s.rp.CategoryWeight(ctx, p.GroupID, normalized)
			wt = w
		}
		points = float64(*p.Minutes) * wt
	case string(repo.KindPurchase):
		if p.AmountYen == nil || *p.AmountYen <= 0 {
			return errors.New("amount_yen required (>0)")
		}
		points = float64(*p.AmountYen) / 10.0 // 仮ルール：10円=1pt
	default:
		return errors.New("invalid type")
	}

	return s.rp.InsertEvent(ctx, repo.InsertEventParams{
		ExtGroupID:  p.GroupID,
		ExtUserID:   p.UserID,
		Kind:        repo.EventKind(p.Type),
		Category:    p.Category,
		Minutes:     p.Minutes,
		AmountYen:   p.AmountYen,
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