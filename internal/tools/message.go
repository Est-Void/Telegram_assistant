package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// SendMessageArgs are the arguments the agent produces for the send_message tool.
type SendMessageArgs struct {
	// Text is the message content.
	Text string `json:"text"`
	// ReplyToMessageID is the ID of the message to reply to (the user's or the
	// bot's own message). When omitted, a regular message is sent.
	ReplyToMessageID *int `json:"reply_to_message_id"`
}

// SendMessageTool returns the tool descriptor (name, description, JSON schema)
// that the agent will use to decide how to answer.
func SendMessageTool() Tool {
	return Tool{
		Name:        "send_message",
		Description: "Send a regular message or a reply to the user's or the bot's own message in the current chat.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "Message text to send.",
				},
				"reply_to_message_id": map[string]any{
					"type":        "integer",
					"description": "ID of the message to reply to (user's or bot's own). Omit to send a regular message.",
				},
			},
			"required": []string{"text"},
		},
	}
}

// ExecuteSendMessage parses the agent's JSON-like arguments and sends the
// message (or reply) through the messenger.
func ExecuteSendMessage(ctx context.Context, deps Deps, raw json.RawMessage) error {
	var args SendMessageArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return fmt.Errorf("parse send_message args: %w", err)
	}

	if args.Text == "" {
		return ErrTextRequired
	}

	if args.ReplyToMessageID != nil && *args.ReplyToMessageID <= 0 {
		return fmt.Errorf("send_message: reply_to_message_id must be positive")
	}

	if err := deps.sleep(ctx, TypingDelay(args.Text, deps.TypingSpeed)); err != nil {
		return fmt.Errorf("send_message: wait before send: %w", err)
	}

	params := SendMessageParams{
		ChatID:               deps.ChatID,
		Text:                 args.Text,
		BusinessConnectionID: deps.BusinessConnectionID,
	}
	if args.ReplyToMessageID != nil {
		params.ReplyToMessageID = *args.ReplyToMessageID
	}

	if err := deps.Messenger.SendMessage(ctx, params); err != nil {
		return fmt.Errorf("send_message: %w", err)
	}

	return nil
}
