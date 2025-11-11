package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"expvar"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"chores_contributor/internal/repo"
	"chores_contributor/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type errResp struct {
	Error string `json:"error"`
}

type lineWebhookPayload struct {
	Destination string      `json:"destination"`
	Events      []lineEvent `json:"events"`
}

type lineEvent struct {
	Type    string      `json:"type"`
	Source  lineSource  `json:"source"`
	Message lineMessage `json:"message"`
}

type lineSource struct {
	Type    string `json:"type"`
	GroupID string `json:"groupId"`
	RoomID  string `json:"roomId"`
	UserID  string `json:"userId"`
}

type lineMessage struct {
	ID      string       `json:"id"`
	Type    string       `json:"type"`
	Text    string       `json:"text"`
	Mention *lineMention `json:"mention,omitempty"`
}

type lineMention struct {
	Mentionees []struct {
		Type   string `json:"type,omitempty"`
		UserID string `json:"userId,omitempty"`
	} `json:"mentionees"`
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(errResp{Error: msg})
}

func requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Admin-Token")
		expected := os.Getenv("ADMIN_TOKEN")
		if expected == "" {
			writeErr(w, 500, "server misconfigured")
			return
		}
		if token != expected {
			writeErr(w, 403, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// verifyLINE LINE署名を検証（HMAC-SHA256）
func verifyLINE(sig string, body []byte, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	calc := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(calc), []byte(sig))
}

// handleLineMessage LINEメッセージを家事報告に変換
func handleLineMessage(ctx context.Context, sv *service.Service, botID string, e lineEvent) {
	isGroupContext := e.Source.GroupID != "" || e.Source.RoomID != ""
	if isGroupContext {
		mentioned := false
		if e.Message.Mention != nil {
			for _, m := range e.Message.Mention.Mentionees {
				if m.UserID == botID {
					mentioned = true
					break
				}
			}
		}
		if !mentioned {
			return
		}
	}

	// 例: "@bot 皿洗い" → task="皿洗い"
	rawFields := strings.Fields(e.Message.Text)
	fields := make([]string, 0, len(rawFields))
	for _, f := range rawFields {
		if strings.HasPrefix(f, "@") {
			continue
		}
		fields = append(fields, f)
	}
	if len(fields) == 0 {
		return
	}

	cmd := strings.ToLower(fields[0])
	switch cmd {
	case "me":
		handleLineCommandMe(ctx, sv, e)
		return
	case "top":
		handleLineCommandTop(ctx, sv, e)
		return
	case "help":
		handleLineCommandHelp(ctx, sv, e)
		return
	}

	task := fields[0]
	var option *string
	if len(fields) > 1 {
		opt := fields[1]
		option = &opt
	}

	groupID := e.Source.GroupID
	if groupID == "" {
		if e.Source.RoomID != "" {
			groupID = e.Source.RoomID
		} else {
			groupID = e.Source.UserID
		}
	}

	payload := service.ReportPayload{
		GroupID:     groupID,
		UserID:      e.Source.UserID,
		Task:        task,
		Option:      option,
		SourceMsgID: &e.Message.ID,
	}

	if err := sv.Report(ctx, payload); err != nil {
		// エラーログ出力（goroutine内なので標準ログを使用）
		log.Printf("LINE webhook error: group=%s user=%s msg_id=%s error=%v",
			groupID, e.Source.UserID, e.Message.ID, err)
		return
	}
}

func handleLineCommandMe(ctx context.Context, sv *service.Service, e lineEvent) {
	groupID := e.Source.GroupID
	if groupID == "" {
		groupID = e.Source.UserID
	}
	log.Printf("LINE command @bot me received: group=%s user=%s", groupID, e.Source.UserID)
}

func handleLineCommandTop(ctx context.Context, sv *service.Service, e lineEvent) {
	groupID := e.Source.GroupID
	if groupID == "" {
		groupID = e.Source.UserID
	}
	log.Printf("LINE command @bot top received: group=%s user=%s", groupID, e.Source.UserID)
}

func handleLineCommandHelp(ctx context.Context, sv *service.Service, e lineEvent) {
	groupID := e.Source.GroupID
	if groupID == "" {
		groupID = e.Source.UserID
	}
	log.Printf("LINE command @bot help received: group=%s user=%s", groupID, e.Source.UserID)
}

func Router(sv *service.Service) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// ルートパス: ブラウザアクセス時の404を防ぐ
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("chores-contributor API is running"))
	})

	// favicon: ログを静かにする
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// ヘルスチェック: 無条件で200を返す（起動直後でも確実に通す）
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// レディネスチェック: DBが準備できたか外部監視用
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
		defer cancel()
		if err := sv.Rp().Ping(ctx); err != nil {
			log.Printf("readyz ping failed: %v", err)
			http.Error(w, "db not ready: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// メトリクスエンドポイント
	r.Mount("/debug/vars", expvar.Handler())

	// LINE Webhook
	// POST /webhook
	r.Post("/webhook", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if !verifyLINE(r.Header.Get("X-Line-Signature"), body, os.Getenv("LINE_CHANNEL_SECRET")) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		var payload lineWebhookPayload

		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		for _, e := range payload.Events {
			if e.Type == "message" && e.Message.Type == "text" {
				eventCopy := e
				go handleLineMessage(r.Context(), sv, payload.Destination, eventCopy)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	})

	// 家事の報告（HTTP版）
	// POST /events/report
	// { "group_id": "default-house", "user_id": "u1", "task": "皿洗い", "source_msg_id": "abc" }
	r.Post("/events/report", func(w http.ResponseWriter, r *http.Request) {
		// Content-Typeガード
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			writeErr(w, 400, "content-type must be application/json")
			return
		}

		var p service.ReportPayload
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&p); err != nil {
			writeErr(w, 400, "invalid json: "+err.Error())
			return
		}
		if err := sv.Report(r.Context(), p); err != nil {
			if errors.Is(err, repo.ErrDuplicateEvent) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status":        "duplicate",
					"source_msg_id": p.SourceMsgID,
				})
				return
			}
			var amb *service.TaskAmbiguousError
			switch {
			case errors.Is(err, service.ErrTaskNotFound):
				writeErr(w, 400, "unknown task")
				return
			case errors.As(err, &amb):
				writeErr(w, 400, "ambiguous task: "+strings.Join(amb.Candidates, ", "))
				return
			case errors.Is(err, service.ErrTaskAmbiguous):
				writeErr(w, 400, "ambiguous task")
				return
			}
			writeErr(w, 400, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// 週次集計（JSON）
	// GET /houses/{group}/weekly?date=2025-11-10  ← date含む週(月曜起点)を集計
	r.Get("/houses/{group}/weekly", func(w http.ResponseWriter, r *http.Request) {
		group := chi.URLParam(r, "group")
		dateStr := r.URL.Query().Get("date")

		ref := time.Now()
		if dateStr != "" {
			if t, err := time.Parse("2006-01-02", dateStr); err == nil {
				ref = t
			}
		}
		wd := int(ref.Weekday())
		if wd == 0 {
			wd = 7
		}
		start := time.Date(ref.Year(), ref.Month(), ref.Day(), 0, 0, 0, 0, ref.Location()).AddDate(0, 0, -(wd - 1))
		end := start.AddDate(0, 0, 7)

		rows, err := sv.Rp().WeeklyPoints(r.Context(), group, start, end)
		if err != nil {
			http.Error(w, "query error", 500)
			return
		}
		out := struct {
			Start string                   `json:"start"`
			End   string                   `json:"end"`
			Rows  []map[string]interface{} `json:"rows"`
		}{
			Start: start.Format("2006-01-02"),
			End:   end.Format("2006-01-02"),
			Rows:  make([]map[string]interface{}, 0, len(rows)),
		}
		for i, x := range rows {
			out.Rows = append(out.Rows, map[string]interface{}{
				"rank":   i + 1,
				"name":   x.Name,
				"points": x.Points,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})

	// カテゴリ重みの編集（管理者用）
	// PUT /admin/houses/{group}/categories/{name}
	// { "weight": 1.5 }
	admin := chi.NewRouter()
	admin.Use(requireAdmin)
	admin.Put("/houses/{group}/categories/{name}", func(w http.ResponseWriter, r *http.Request) {
		group := chi.URLParam(r, "group")
		name := chi.URLParam(r, "name")
		if group == "" || name == "" {
			writeErr(w, 400, "group and name are required")
			return
		}
		var body struct {
			Weight float64 `json:"weight"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			writeErr(w, 400, "invalid json: "+err.Error())
			return
		}
		if body.Weight <= 0 {
			writeErr(w, 400, "weight must be greater than 0")
			return
		}
		if err := sv.UpsertCategory(r.Context(), group, name, body.Weight); err != nil {
			if errors.Is(err, repo.ErrHouseNotFound) {
				writeErr(w, 404, "house not found")
				return
			}
			writeErr(w, 500, "update failed: "+err.Error())
			return
		}
		w.WriteHeader(204)
	})
	r.Mount("/admin", admin)

	return r
}
