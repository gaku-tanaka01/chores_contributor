package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"io"
	"log"
	"math"
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
	Type       string      `json:"type"`
	ReplyToken string      `json:"replyToken"`
	Source     lineSource  `json:"source"`
	Message    lineMessage `json:"message"`
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

type lineReplyMessage struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type lineReplyRequest struct {
	ReplyToken string             `json:"replyToken"`
	Messages   []lineReplyMessage `json:"messages"`
}

const lineReplyEndpoint = "https://api.line.me/v2/bot/message/reply"

func sendLineReply(ctx context.Context, replyToken string, texts ...string) error {
	if replyToken == "" {
		return errors.New("empty reply token")
	}
	token := os.Getenv("LINE_CHANNEL_ACCESS_TOKEN")
	if token == "" {
		return errors.New("LINE_CHANNEL_ACCESS_TOKEN not set")
	}

	msgs := make([]lineReplyMessage, 0, len(texts))
	for _, t := range texts {
		if strings.TrimSpace(t) == "" {
			continue
		}
		r := []rune(t)
		if len(r) > 1000 {
			r = r[:1000]
		}
		msgs = append(msgs, lineReplyMessage{Type: "text", Text: string(r)})
	}
	if len(msgs) == 0 {
		return nil
	}

	payload, err := json.Marshal(lineReplyRequest{ReplyToken: replyToken, Messages: msgs})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, lineReplyEndpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("line reply failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(errResp{Error: msg})
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

	groupID := e.Source.GroupID
	if groupID == "" {
		if e.Source.RoomID != "" {
			groupID = e.Source.RoomID
		} else {
			groupID = e.Source.UserID
		}
	}

	cmd := strings.ToLower(fields[0])
	switch cmd {
	case "me":
		summary, err := sv.WeeklyUserSummary(ctx, groupID, e.Source.UserID, time.Now())
		if err != nil {
			if replyErr := sendLineReply(ctx, e.ReplyToken, "取得失敗: 少し待ってから試してね"); replyErr != nil {
				log.Printf("LINE reply error (me command failure): %v", replyErr)
			}
			log.Printf("LINE summary error: group=%s user=%s error=%v", groupID, e.Source.UserID, err)
			return
		}
		if len(summary.TaskList) == 0 {
			if err := sendLineReply(ctx, e.ReplyToken, "今週のポイントはまだ0ptだよ。"); err != nil {
				log.Printf("LINE reply error (me command zero): %v", err)
			}
			return
		}
		breakdown := make([]string, 0, len(summary.TaskList))
		for _, item := range summary.TaskList {
			breakdown = append(breakdown, fmt.Sprintf("%s:%s", item.TaskKey, formatPoints(item.Points)))
		}
		msg := fmt.Sprintf("今週:%s (%s)", formatPoints(summary.Total), strings.Join(breakdown, ", "))
		if err := sendLineReply(ctx, e.ReplyToken, msg); err != nil {
			log.Printf("LINE reply error (me command): %v", err)
		}
		return
	case "取消", "取り消し", "キャンセル", "cancel":
		result, err := sv.CancelLatestEvent(ctx, groupID, e.Source.UserID)
		if err != nil {
			msg := "取り消し失敗: 少し待ってね"
			if errors.Is(err, repo.ErrNoEventFound) {
				msg = "取り消す記録がないよ。"
			}
			if replyErr := sendLineReply(ctx, e.ReplyToken, msg); replyErr != nil {
				log.Printf("LINE reply error (cancel failure): %v", replyErr)
			}
			return
		}
		if err := sendLineReply(ctx, e.ReplyToken, fmt.Sprintf("直前の「%s」を取り消したよ。", result.TaskKey)); err != nil {
			log.Printf("LINE reply error (cancel success): %v", err)
		}
		return
	case "top":
		reply := "今週のランキング表示は準備中だよ。もう少し待ってて！"
		if err := sendLineReply(ctx, e.ReplyToken, reply); err != nil {
			log.Printf("LINE reply error (top command): %v", err)
		}
		return
	case "help":
		helpText := strings.Join([]string{
			"使い方:",
			"・@bot 皿洗い → 家事報告",
			"・@bot me → 今週の自分のポイント",
			"・@bot top → 今週のTOP3 (準備中)",
			"・@bot 取消 → 直前の報告を取り消す",
			"・@bot help → このメッセージ",
			"タスク名はかな/英語/タイプミス1文字まで自動補正するよ。",
		}, "\n")
		if err := sendLineReply(ctx, e.ReplyToken, helpText); err != nil {
			log.Printf("LINE reply error (help command): %v", err)
		}
		return
	}

	task := fields[0]
	var option *string
	if len(fields) > 1 {
		opt := fields[1]
		option = &opt
	}

	payload := service.ReportPayload{
		GroupID:     groupID,
		UserID:      e.Source.UserID,
		Task:        task,
		Option:      option,
		SourceMsgID: &e.Message.ID,
	}

	if err := sv.Report(ctx, payload); err != nil {
		var amb *service.TaskAmbiguousError
		var msg string
		switch {
		case errors.Is(err, repo.ErrDuplicateEvent):
			msg = "重複: この報告は登録済みだよ"
		case errors.Is(err, service.ErrTaskNotFound):
			msg = fmt.Sprintf("不明: \"%s\"", task)
		case errors.As(err, &amb):
			msg = fmt.Sprintf("不明: \"%s\" 候補: %s", task, strings.Join(amb.Candidates, "/"))
		default:
			log.Printf("LINE webhook error: group=%s user=%s msg_id=%s error=%v", groupID, e.Source.UserID, e.Message.ID, err)
			msg = "失敗: 少し待ってから試してね"
		}
		if replyErr := sendLineReply(ctx, e.ReplyToken, msg); replyErr != nil {
			log.Printf("LINE reply error (failure notice): %v", replyErr)
		}
		return
	}
}

func formatPoints(pt float64) string {
	if math.Abs(pt-math.Round(pt)) < 1e-6 {
		return fmt.Sprintf("%.0fpt", math.Round(pt))
	}
	return fmt.Sprintf("%.1fpt", pt)
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

	lineSecret := os.Getenv("LINE_CHANNEL_SECRET")
	if lineSecret == "" {
		log.Println("LINE_CHANNEL_SECRET length=0 (empty)")
	} else {
		log.Printf("LINE_CHANNEL_SECRET length=%d", len(lineSecret))
	}
	lineToken := os.Getenv("LINE_CHANNEL_ACCESS_TOKEN")
	if lineToken == "" {
		log.Println("LINE_CHANNEL_ACCESS_TOKEN length=0 (empty)")
	} else {
		log.Printf("LINE_CHANNEL_ACCESS_TOKEN length=%d", len(lineToken))
	}

	// LINE Webhook
	// POST /webhook
	r.Post("/webhook", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

	sig := r.Header.Get("X-Line-Signature")
	secret := os.Getenv("LINE_CHANNEL_SECRET")
	if !verifyLINE(sig, body, secret) {
		log.Printf("LINE signature mismatch: headerLen=%d bodyLen=%d secretLen=%d", len(sig), len(body), len(secret))
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
				go func(ev lineEvent) {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					handleLineMessage(ctx, sv, payload.Destination, ev)
				}(eventCopy)
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

	return r
}
