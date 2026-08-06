package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"telegram-assistant/internal/cache"
	"telegram-assistant/internal/chat"
	"telegram-assistant/internal/tools"
	"telegram-assistant/internal/userbot"
)

type fakeMessenger struct {
	messageCalls []tools.SendMessageParams
	stickerCalls []tools.SendStickerParams
}

func (f *fakeMessenger) SendMessage(_ context.Context, p tools.SendMessageParams) error {
	f.messageCalls = append(f.messageCalls, p)
	return nil
}

func (f *fakeMessenger) SendSticker(_ context.Context, p tools.SendStickerParams) error {
	f.stickerCalls = append(f.stickerCalls, p)
	return nil
}

type fakeSource struct {
	history []userbot.Message
}

func (s *fakeSource) History(_ context.Context, _ int64, _ int) ([]userbot.Message, error) {
	return s.history, nil
}

func (s *fakeSource) Profile(_ context.Context, id int64) (*userbot.UserProfile, error) {
	return &userbot.UserProfile{ID: id, FirstName: "Test"}, nil
}

func newTestAgent(t *testing.T, srvURL string, src chat.Source) (*Agent, *fakeMessenger) {
	t.Helper()

	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := chat.New(src, cache.NewMemory())

	a := New(Options{
		Model:       "test-model",
		BaseURL:     srvURL,
		Chat:        svc,
		Catalog:     &tools.StickerCatalog{Stickers: []tools.StickerItem{{Key: "hi", FileID: "fid"}}},
		TypingSpeed: 0,
		Logger:      discard,
	})
	return a, &fakeMessenger{}
}

func newOllamaServer(t *testing.T, responses []ollamaResponse) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		if len(responses) == 0 {
			t.Error("unexpected ollama request")
			http.Error(w, "unexpected request", http.StatusInternalServerError)
			return
		}
		resp := responses[0]
		responses = responses[1:]
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func toolCall(id, name string, args any) ollamaResponse {
	raw, _ := json.Marshal(args)
	return ollamaResponse{Message: ollamaMessage{
		Role: "assistant",
		ToolCalls: []ollamaToolCall{{
			ID: id,
			Function: ollamaFunctionCall{
				Name:      name,
				Arguments: raw,
			},
		}},
	}}
}

func TestInvokeSendsViaToolAndDropsFinalContent(t *testing.T) {
	srv := newOllamaServer(t, []ollamaResponse{
		toolCall("tc-1", "send_message", map[string]any{"text": "Привет!"}),
		{Message: ollamaMessage{Role: "assistant", Content: "Привет!"}},
	})

	ag, msgr := newTestAgent(t, srv.URL, &fakeSource{})

	conv := []ollamaMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "Привет"},
	}
	deps := tools.Deps{ChatID: 123, Messenger: msgr, Chat: ag.chat}

	if err := ag.invoke(context.Background(), deps, conv, 42); err != nil {
		t.Fatalf("invoke: %v", err)
	}

	if len(msgr.messageCalls) != 1 {
		t.Fatalf("expected exactly 1 sent message, got %d", len(msgr.messageCalls))
	}
	got := msgr.messageCalls[0]
	if got.Text != "Привет!" {
		t.Errorf("message text = %q, want %q", got.Text, "Привет!")
	}
	if got.ReplyToMessageID != 0 {
		t.Errorf("tool send should not reply, got reply_to=%d", got.ReplyToMessageID)
	}
}

func TestInvokeSendsFinalContentWithoutTools(t *testing.T) {
	srv := newOllamaServer(t, []ollamaResponse{
		{Message: ollamaMessage{Role: "assistant", Content: "Как дела?"}},
	})

	ag, msgr := newTestAgent(t, srv.URL, &fakeSource{})
	deps := tools.Deps{ChatID: 123, Messenger: msgr, Chat: ag.chat}

	conv := []ollamaMessage{{Role: "user", Content: "Как ты?"}}
	if err := ag.invoke(context.Background(), deps, conv, 7); err != nil {
		t.Fatalf("invoke: %v", err)
	}

	if len(msgr.messageCalls) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(msgr.messageCalls))
	}
	got := msgr.messageCalls[0]
	if got.Text != "Как дела?" {
		t.Errorf("message text = %q, want %q", got.Text, "Как дела?")
	}
	if got.ReplyToMessageID != 7 {
		t.Errorf("reply_to_message_id = %d, want 7", got.ReplyToMessageID)
	}
}

func TestInvokeHistoryToolThenFinalAnswer(t *testing.T) {
	src := &fakeSource{history: []userbot.Message{
		{ID: 1, SenderID: 999, SenderName: "Иван", Text: "Первое", Date: 100},
		{ID: 2, SenderID: 999, SenderName: "Иван", Text: "Второе", Date: 200},
	}}

	srv := newOllamaServer(t, []ollamaResponse{
		toolCall("tc-2", "get_chat_history", map[string]any{"chat_id": 123, "limit": 2}),
		{Message: ollamaMessage{Role: "assistant", Content: "Вот история"}},
	})

	ag, msgr := newTestAgent(t, srv.URL, src)
	deps := tools.Deps{ChatID: 123, Messenger: msgr, Chat: ag.chat}

	conv := []ollamaMessage{{Role: "user", Content: "Покажи историю"}}
	if err := ag.invoke(context.Background(), deps, conv, 0); err != nil {
		t.Fatalf("invoke: %v", err)
	}

	if len(msgr.messageCalls) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(msgr.messageCalls))
	}
	if msgr.messageCalls[0].Text != "Вот история" {
		t.Errorf("message text = %q, want %q", msgr.messageCalls[0].Text, "Вот история")
	}
}

func TestInvokeUnknownToolReportsError(t *testing.T) {
	srv := newOllamaServer(t, []ollamaResponse{
		toolCall("tc-3", "make_coffee", map[string]any{}),
		{Message: ollamaMessage{Role: "assistant", Content: "Ок"}},
	})

	ag, msgr := newTestAgent(t, srv.URL, &fakeSource{})
	deps := tools.Deps{ChatID: 123, Messenger: msgr, Chat: ag.chat}

	conv := []ollamaMessage{{Role: "user", Content: "Свари кофе"}}
	if err := ag.invoke(context.Background(), deps, conv, 0); err != nil {
		t.Fatalf("invoke: %v", err)
	}

	if len(msgr.messageCalls) != 1 {
		t.Fatalf("expected final message despite tool error, got %d", len(msgr.messageCalls))
	}
}

func TestStripThinking(t *testing.T) {
	cases := map[string]string{
		"plain":                          "plain",
		"<thinking>hmm</thinking>answer": "answer",
		"＜thinking＞хм＜/thinking＞ответ":                   "ответ",
		"<thinking>a</thinking>b<thinking>c</thinking>d": "bd",
	}
	for in, want := range cases {
		if got := stripThinking(in); got != want {
			t.Errorf("stripThinking(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeArgs(t *testing.T) {
	if got := string(normalizeArgs(json.RawMessage(`"{\"city\":\"Moscow\"}"`))); got != `{"city":"Moscow"}` {
		t.Errorf("stringified args not normalized: %s", got)
	}
	if got := string(normalizeArgs(json.RawMessage(`{"city":"Moscow"}`))); got != `{"city":"Moscow"}` {
		t.Errorf("object args mangled: %s", got)
	}
	if got := string(normalizeArgs(nil)); got != "{}" {
		t.Errorf("empty args = %s, want {}", got)
	}
}
