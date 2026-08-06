package tools

import (
	"context"
	"errors"
	"time"

	"telegram-assistant/internal/userbot"
)

// Messenger abstracts the Telegram send API so tools can be unit-tested
// without a live bot connection.
type Messenger interface {
	SendMessage(ctx context.Context, params SendMessageParams) error
	SendSticker(ctx context.Context, params SendStickerParams) error
}

// ChatSource provides read access to chat history and user profiles.
type ChatSource interface {
	History(ctx context.Context, chatID int64, limit int) ([]userbot.Message, error)
	Profile(ctx context.Context, userID int64) (*userbot.UserProfile, error)
}

// SendMessageParams is the translation of a tool call into a send request.
type SendMessageParams struct {
	ChatID               int64
	Text                 string
	ReplyToMessageID     int
	BusinessConnectionID string
}

// SendStickerParams is the translation of a sticker tool call into a request.
type SendStickerParams struct {
	ChatID               int64
	FileID               string
	ReplyToMessageID     int
	BusinessConnectionID string
}

// Deps carries per-execution context injected by the integration layer.
type Deps struct {
	ChatID               int64
	BusinessConnectionID string
	Messenger            Messenger
	// TypingSpeed is the agent's typing speed in characters per second.
	// It drives the len(text)/speed delay before a message is sent.
	TypingSpeed float64
	// Sleep overrides the wait before sending (defaults to a cancellable sleep).
	Sleep func(ctx context.Context, d time.Duration) error
	// Schedule overrides delayed task execution (defaults to time.AfterFunc).
	Schedule func(ctx context.Context, delay time.Duration, task func(ctx context.Context)) error
	// ReinvokeAgent re-runs the agent for the current conversation. It is
	// required for the later tool's call_agent action.
	ReinvokeAgent func(ctx context.Context) error
	// Chat provides read access to chat history and user profiles
	// (get_chat_history, get_user_profile tools).
	Chat ChatSource
}

// Tool describes a tool the agent can call.
type Tool struct {
	Name        string
	Description string
	Parameters  any
}

var (
	ErrTextRequired       = errors.New("send_message: text is required")
	ErrStickerKeyRequired = errors.New("send_sticker: sticker key is required")
)
