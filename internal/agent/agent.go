// Package agent runs an Ollama-powered assistant that answers business
// messages and can use tools to send messages, look up chat history and
// user profiles, send stickers and schedule later actions.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"telegram-assistant/internal/chat"
	"telegram-assistant/internal/tools"
	"telegram-assistant/internal/userbot"
)

const (
	seedMessages  = 8
	maxIterations = 10
)

var (
	thinkingASCII = regexp.MustCompile(`(?s)<thinking>.*?</thinking>`)
	thinkingFull  = regexp.MustCompile(`(?s)＜thinking＞.*?＜/thinking＞`)
)

// Options configures the agent.
type Options struct {
	// Model is the Ollama model name to use.
	Model string
	// BaseURL is the Ollama server base URL, e.g. http://localhost:11434.
	BaseURL string
	// Chat gives the agent read access to chat history and user profiles.
	Chat *chat.Service
	// Catalog provides the stickers the agent may send.
	Catalog *tools.StickerCatalog
	// TypingSpeed drives the send delay (characters per second).
	TypingSpeed float64
	// Logger receives agent logs.
	Logger *slog.Logger
}

// Agent is an Ollama-backed conversational assistant.
type Agent struct {
	model       string
	baseURL     string
	http        *http.Client
	chat        *chat.Service
	catalog     *tools.StickerCatalog
	typingSpeed float64
	log         *slog.Logger
	tools       []map[string]any
}

// New creates an agent.
func New(opts Options) *Agent {
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	}

	a := &Agent{
		model:       opts.Model,
		baseURL:     strings.TrimRight(opts.BaseURL, "/"),
		http:        &http.Client{Timeout: 2 * time.Minute},
		chat:        opts.Chat,
		catalog:     opts.Catalog,
		typingSpeed: opts.TypingSpeed,
		log:         log,
	}
	a.tools = a.ollamaTools()
	return a
}

// systemPrompt describes the assistant's role to the model.
func (a *Agent) systemPrompt() string {
	return `Ты — личный ассистент владельца этого Telegram-аккаунта. Ты ведёшь диалог в чате от имени владельца, поэтому обращайся к собеседнику вежливо и по делу.

Правила:
1. Отвечай собеседнику через инструмент send_message. Не описывай свои действия — просто отправь ответ.
2. Пиши только сам текст ответа, без префиксов и имён (не начинай с «я:», «владелец:» и т.п.).
3. Чтобы ответить цитатой, укажи reply_to_message_id равным ID сообщения собеседника.
4. Если нужно вспомнить более раннюю переписку — используй get_chat_history. Чтобы понять, кто пишет — get_user_profile.
5. Стикеры отправляй редко, только если это действительно уместно.
6. later — для отложенных действий (отправить сообщение или вернуться к диалогу позже).
7. Отвечай на языке собеседника.
8. Не выдумывай фактов, которых нет в истории чата.`
}

// ollamaTools converts the tool descriptors into the function-calling schema
// that Ollama expects.
func (a *Agent) ollamaTools() []map[string]any {
	descriptors := []tools.Tool{
		tools.SendMessageTool(),
		tools.GetChatHistoryTool(),
		tools.GetUserProfileTool(),
		tools.LaterTool(),
	}
	if a.catalog != nil {
		descriptors = append(descriptors, tools.SendStickerTool(a.catalog))
	}

	out := make([]map[string]any, 0, len(descriptors))
	for _, t := range descriptors {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			},
		})
	}
	return out
}

// Handle processes a single incoming business message through the agent.
func (a *Agent) Handle(ctx context.Context, b *bot.Bot, msg *models.Message) {
	a.HandleChunk(ctx, b, []*models.Message{msg})
}

// HandleChunk processes a batch of business messages (see Buffer): builds the
// conversation context, lets the model answer (possibly using tools) and
// delivers the reply.
func (a *Agent) HandleChunk(ctx context.Context, b *bot.Bot, msgs []*models.Message) {
	if len(msgs) == 0 {
		return
	}
	for _, m := range msgs {
		a.log.Info("agent message received",
			"connection_id", m.BusinessConnectionID,
			"user_id", m.From.ID,
			"username", m.From.Username,
			"chat_id", m.Chat.ID,
			"message_id", m.ID,
			"text", m.Text,
		)
	}

	chatID := msgs[0].Chat.ID
	conv, err := a.buildContext(ctx, chatID, msgs)
	if err != nil {
		a.log.Warn("agent: building context failed", "chat_id", chatID, "error", err)
	}
	if len(conv) == 0 {
		conv = []ollamaMessage{{
			Role:    "system",
			Content: a.systemPrompt(),
		}}
	}
	a.log.Debug("agent: conversation built", "chat_id", chatID, "turns", len(conv)-1)

	deps := tools.Deps{
		ChatID:               chatID,
		BusinessConnectionID: msgs[len(msgs)-1].BusinessConnectionID,
		Messenger:            tools.NewBotMessenger(b),
		TypingSpeed:          a.typingSpeed,
		Chat:                 a.chat,
	}
	deps.ReinvokeAgent = func(ctx context.Context) error {
		conv, err := a.buildContext(ctx, deps.ChatID, nil)
		if err != nil {
			return err
		}
		return a.invoke(ctx, deps, conv, 0)
	}

	replyToID := msgs[len(msgs)-1].ID
	if err := a.invoke(ctx, deps, conv, replyToID); err != nil {
		a.log.Error("agent: invoke failed",
			"chat_id", chatID, "message_id", replyToID, "error", err)
	}
}

// buildContext assembles the system prompt and the recent history of the chat
// (oldest first). The incoming messages are appended explicitly unless they
// are already present in the history.
func (a *Agent) buildContext(ctx context.Context, chatID int64, incoming []*models.Message) ([]ollamaMessage, error) {
	hist, err := a.chat.History(ctx, chatID, seedMessages)
	if err != nil {
		return nil, fmt.Errorf("agent: load history: %w", err)
	}

	conv := make([]ollamaMessage, 0, seedMessages+len(incoming)+1)
	conv = append(conv, ollamaMessage{Role: "system", Content: a.systemPrompt()})

	for i := len(hist) - 1; i >= 0; i-- {
		conv = append(conv, a.frameMessage(hist[i]))
	}

	for _, m := range incoming {
		found := false
		for _, h := range hist {
			if h.ID == m.ID {
				found = true
				break
			}
		}
		if !found {
			conv = append(conv, a.incomingMessage(m))
		}
	}

	return conv, nil
}

func (a *Agent) frameMessage(m userbot.Message) ollamaMessage {
	text := strings.TrimSpace(m.Text)
	if text == "" {
		text = "(медиа)"
	}
	if m.Out {
		// The out-role already conveys these are the owner's words; a
		// prefix would leak into the model's replies.
		return ollamaMessage{Role: "assistant", Content: text}
	}
	name := m.SenderName
	if name == "" {
		name = fmt.Sprintf("user-%d", m.SenderID)
	}
	return ollamaMessage{Role: "user", Content: name + ": " + text}
}

func (a *Agent) incomingMessage(msg *models.Message) ollamaMessage {
	from := msg.From.Username
	if from == "" {
		from = msg.From.FirstName
	}
	if from == "" {
		from = fmt.Sprintf("user-%d", msg.From.ID)
	}
	text := msg.Text
	if text == "" {
		text = "(медиа)"
	}
	return ollamaMessage{
		Role:    "user",
		Content: fmt.Sprintf("[ID %d] %s: %s", msg.ID, from, text),
	}
}

// invoke runs the model loop: ask Ollama, execute any tool calls, repeat,
// and deliver the final text answer. The answer is sent once: either via the
// send_message/send_sticker tools or as final content, never both.
func (a *Agent) invoke(ctx context.Context, deps tools.Deps, conv []ollamaMessage, replyToID int) error {
	msgs := conv
	var sent bool

	for i := 0; i < maxIterations; i++ {
		resp, err := a.ollamaChat(ctx, msgs)
		if err != nil {
			return err
		}

		assistant := resp.Message
		if len(assistant.ToolCalls) == 0 {
			text := stripThinking(assistant.Content)
			if strings.TrimSpace(text) == "" {
				a.log.Warn("agent: model returned empty response",
					"thinking", truncate(assistant.Thinking, 200))
				return nil
			}
			if sent {
				a.log.Debug("agent: dropping final content, reply already sent", "text", truncate(text, 200))
				return nil
			}
			a.log.Debug("agent: final content", "text", truncate(text, 200))
			return a.sendFinal(ctx, deps, text, replyToID)
		}

		a.log.Debug("agent: tool calls", "count", len(assistant.ToolCalls))
		msgs = append(msgs, ollamaMessage{
			Role:      "assistant",
			Content:   assistant.Content,
			ToolCalls: assistant.ToolCalls,
		})
		for _, tc := range assistant.ToolCalls {
			result, isSend := a.dispatch(ctx, deps, tc.Function.Name, tc.Function.Arguments)
			if isSend {
				sent = true
			}
			a.log.Debug("agent: tool result",
				"tool", tc.Function.Name,
				"result", truncate(result, 200),
			)
			msgs = append(msgs, ollamaMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    result,
			})
		}
	}

	return errors.New("too many tool call iterations")
}

// sendFinal delivers the agent's final text answer, replying to the incoming
// message when possible. The typing delay applies like in the send_message tool.
func (a *Agent) sendFinal(ctx context.Context, deps tools.Deps, text string, replyToID int) error {
	args := tools.SendMessageArgs{Text: text}
	if replyToID > 0 {
		args.ReplyToMessageID = &replyToID
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("agent: marshal final answer: %w", err)
	}
	if err := tools.ExecuteSendMessage(ctx, deps, raw); err != nil {
		return fmt.Errorf("agent: send final answer: %w", err)
	}
	a.log.Info("agent: reply sent", "chat_id", deps.ChatID, "reply_to", replyToID, "text", truncate(text, 200))
	return nil
}

// dispatch executes a single tool call and returns the result to feed back
// to the model. The second return value reports whether the call delivered
// a reply to the chat (send_message / send_sticker).
func (a *Agent) dispatch(ctx context.Context, deps tools.Deps, name string, raw json.RawMessage) (result string, isSend bool) {
	raw = normalizeArgs(raw)

	switch name {
	case "send_message":
		if err := tools.ExecuteSendMessage(ctx, deps, raw); err != nil {
			return "error: " + err.Error(), false
		}
		return "sent", true
	case "send_sticker":
		if a.catalog == nil {
			return "error: send_sticker is not available", false
		}
		if err := tools.ExecuteSendSticker(ctx, deps, a.catalog, raw); err != nil {
			return "error: " + err.Error(), false
		}
		return "sent", true
	case "get_chat_history":
		res, err := tools.ExecuteGetChatHistory(ctx, deps.Chat, raw)
		if err != nil {
			return "error: " + err.Error(), false
		}
		return res, false
	case "get_user_profile":
		res, err := tools.ExecuteGetUserProfile(ctx, deps.Chat, raw)
		if err != nil {
			return "error: " + err.Error(), false
		}
		return res, false
	case "later":
		if err := tools.ExecuteLater(ctx, deps, raw); err != nil {
			return "error: " + err.Error(), false
		}
		return "scheduled", false
	default:
		return fmt.Sprintf("error: unknown tool %q", name), false
	}
}

// ollamaChat is a single non-streaming request to the Ollama chat API.
func (a *Agent) ollamaChat(ctx context.Context, msgs []ollamaMessage) (*ollamaResponse, error) {
	req := ollamaRequest{
		Model:    a.model,
		Messages: msgs,
		Tools:    a.tools,
		Stream:   false,
		Options: map[string]any{
			"temperature": 0.7,
			"num_ctx":     8192,
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("agent: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("agent: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := a.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("agent: ollama request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(httpResp.Body)
		return nil, fmt.Errorf("agent: ollama status %d: %s", httpResp.StatusCode, truncate(buf.String(), 300))
	}

	var resp ollamaResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("agent: decode ollama response: %w", err)
	}
	a.log.Debug("agent: ollama response",
		"done_reason", resp.DoneReason,
		"content_len", len(resp.Message.Content),
		"tool_calls", len(resp.Message.ToolCalls),
	)
	return &resp, nil
}

// ollamaRequest is the payload of the /api/chat endpoint.
type ollamaRequest struct {
	Model    string           `json:"model"`
	Messages []ollamaMessage  `json:"messages"`
	Tools    []map[string]any `json:"tools,omitempty"`
	Stream   bool             `json:"stream"`
	Options  map[string]any   `json:"options,omitempty"`
}

type ollamaResponse struct {
	Message    ollamaMessage `json:"message"`
	DoneReason string        `json:"done_reason"`
}

type ollamaMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	Thinking   string           `json:"thinking,omitempty"`
	ToolCalls  []ollamaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type ollamaToolCall struct {
	ID       string             `json:"id"`
	Function ollamaFunctionCall `json:"function"`
}

type ollamaFunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// normalizeArgs ensures the tool arguments are a JSON object: Ollama usually
// returns an object, but some models emit a JSON string.
func normalizeArgs(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil && json.Valid(json.RawMessage(s)) {
			return json.RawMessage(s)
		}
	}
	return raw
}

// stripThinking removes reasoning blocks that some models (e.g. qwen3)
// emit in the final answer.
func stripThinking(s string) string {
	s = thinkingASCII.ReplaceAllString(s, "")
	s = thinkingFull.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
