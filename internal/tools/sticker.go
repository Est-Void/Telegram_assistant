package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SendStickerArgs are the arguments the agent produces for the send_sticker tool.
type SendStickerArgs struct {
	// Sticker is the catalog key of the sticker to send.
	Sticker string `json:"sticker"`
	// ReplyToMessageID is the ID of the message to reply to. Omit for a regular message.
	ReplyToMessageID *int `json:"reply_to_message_id"`
}

// SendStickerTool returns the tool descriptor. Available sticker keys are
// listed so the agent can pick one.
func SendStickerTool(catalog *StickerCatalog) Tool {
	return Tool{
		Name:        "send_sticker",
		Description: fmt.Sprintf("Send a sticker from the assistant's catalog. Available sticker keys: %s.", strings.Join(catalog.Keys(), ", ")),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sticker": map[string]any{
					"type":        "string",
					"description": fmt.Sprintf("Sticker catalog key. One of: %s.", strings.Join(catalog.Keys(), ", ")),
				},
				"reply_to_message_id": map[string]any{
					"type":        "integer",
					"description": "ID of the message to reply to (user's or bot's own). Omit to send a regular message.",
				},
			},
			"required": []string{"sticker"},
		},
	}
}

// ExecuteSendSticker parses the agent's JSON-like arguments, resolves the
// sticker key in the catalog and sends the sticker (or reply) through the messenger.
func ExecuteSendSticker(ctx context.Context, deps Deps, catalog *StickerCatalog, raw json.RawMessage) error {
	var args SendStickerArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return fmt.Errorf("parse send_sticker args: %w", err)
	}

	if args.Sticker == "" {
		return ErrStickerKeyRequired
	}

	if args.ReplyToMessageID != nil && *args.ReplyToMessageID <= 0 {
		return fmt.Errorf("send_sticker: reply_to_message_id must be positive")
	}

	fileID, ok := catalog.Lookup(args.Sticker)
	if !ok {
		return fmt.Errorf("send_sticker: unknown sticker key %q", args.Sticker)
	}

	params := SendStickerParams{
		ChatID:               deps.ChatID,
		FileID:               fileID,
		BusinessConnectionID: deps.BusinessConnectionID,
	}
	if args.ReplyToMessageID != nil {
		params.ReplyToMessageID = *args.ReplyToMessageID
	}

	if err := deps.Messenger.SendSticker(ctx, params); err != nil {
		return fmt.Errorf("send_sticker: %w", err)
	}

	return nil
}
