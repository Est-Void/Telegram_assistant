package tools

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// BotMessenger adapts a live go-telegram/bot instance to the Messenger
// interface, translating the params into Bot API requests.
type BotMessenger struct {
	b *bot.Bot
}

// NewBotMessenger creates a Messenger backed by a real Telegram bot.
func NewBotMessenger(b *bot.Bot) *BotMessenger {
	return &BotMessenger{b: b}
}

func (m *BotMessenger) SendMessage(ctx context.Context, params SendMessageParams) error {
	req := &bot.SendMessageParams{
		ChatID:               params.ChatID,
		Text:                 params.Text,
		BusinessConnectionID: params.BusinessConnectionID,
	}
	if params.ReplyToMessageID != 0 {
		req.ReplyParameters = &models.ReplyParameters{
			MessageID:                params.ReplyToMessageID,
			AllowSendingWithoutReply: true,
		}
	}

	_, err := m.b.SendMessage(ctx, req)
	return err
}

func (m *BotMessenger) SendSticker(ctx context.Context, params SendStickerParams) error {
	req := &bot.SendStickerParams{
		ChatID:               params.ChatID,
		Sticker:              &models.InputFileString{Data: params.FileID},
		BusinessConnectionID: params.BusinessConnectionID,
	}
	if params.ReplyToMessageID != 0 {
		req.ReplyParameters = &models.ReplyParameters{
			MessageID:                params.ReplyToMessageID,
			AllowSendingWithoutReply: true,
		}
	}

	_, err := m.b.SendSticker(ctx, req)
	return err
}
