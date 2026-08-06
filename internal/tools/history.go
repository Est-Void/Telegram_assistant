package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// GetChatHistoryArgs are the arguments for the get_chat_history tool.
type GetChatHistoryArgs struct {
	// ChatID is the Telegram chat ID to read.
	ChatID int64 `json:"chat_id"`
	// Limit is the number of messages to return, 1..50. Defaults to 20.
	Limit *int `json:"limit"`
}

// DefaultHistoryLimit is used when the agent omits the limit argument.
const DefaultHistoryLimit = 20

// MaxHistoryLimit caps how many messages can be requested at once.
const MaxHistoryLimit = 50

// GetChatHistoryTool returns the tool descriptor for the agent.
func GetChatHistoryTool() Tool {
	return Tool{
		Name:        "get_chat_history",
		Description: "Get the most recent messages of a chat (newest first). Use it to recall what the user wrote earlier in this conversation or other chats.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"chat_id": map[string]any{
					"type":        "integer",
					"description": "Telegram chat ID. The current conversation by default; pass another ID to read another chat.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Number of messages to return, 1..50. Defaults to 20.",
				},
			},
			"required": []string{"chat_id"},
		},
	}
}

// ExecuteGetChatHistory parses the agent's arguments and returns the chat
// history as a JSON array of messages.
func ExecuteGetChatHistory(ctx context.Context, source ChatSource, raw json.RawMessage) (string, error) {
	var args GetChatHistoryArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("parse get_chat_history args: %w", err)
	}

	if args.ChatID == 0 {
		return "", fmt.Errorf("get_chat_history: chat_id is required")
	}

	limit := DefaultHistoryLimit
	if args.Limit != nil {
		limit = *args.Limit
	}
	if limit < 1 || limit > MaxHistoryLimit {
		return "", fmt.Errorf("get_chat_history: limit must be between 1 and %d", MaxHistoryLimit)
	}

	msgs, err := source.History(ctx, args.ChatID, limit)
	if err != nil {
		return "", fmt.Errorf("get_chat_history: %w", err)
	}

	data, err := json.Marshal(msgs)
	if err != nil {
		return "", fmt.Errorf("get_chat_history: marshal result: %w", err)
	}
	return string(data), nil
}
