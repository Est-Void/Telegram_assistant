package tools

import (
	"context"
	"errors"
)

type fakeMessenger struct {
	messageCalls []SendMessageParams
	stickerCalls []SendStickerParams
}

func (f *fakeMessenger) SendMessage(_ context.Context, params SendMessageParams) error {
	f.messageCalls = append(f.messageCalls, params)
	return nil
}

func (f *fakeMessenger) SendSticker(_ context.Context, params SendStickerParams) error {
	f.stickerCalls = append(f.stickerCalls, params)
	return nil
}

type errorMessenger struct{}

func (errorMessenger) SendMessage(context.Context, SendMessageParams) error {
	return errors.New("boom")
}

func (errorMessenger) SendSticker(context.Context, SendStickerParams) error {
	return errors.New("boom")
}
