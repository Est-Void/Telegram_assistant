package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"telegram-assistant/internal/userbot"
)

type fakeChatSource struct {
	history []userbot.Message
	profile *userbot.UserProfile
	err     error
}

type chatSourceFunc func(ctx context.Context, chatID int64, limit int) ([]userbot.Message, error)

func (f chatSourceFunc) History(ctx context.Context, chatID int64, limit int) ([]userbot.Message, error) {
	return f(ctx, chatID, limit)
}

func (chatSourceFunc) Profile(_ context.Context, _ int64) (*userbot.UserProfile, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeChatSource) History(_ context.Context, _ int64, _ int) ([]userbot.Message, error) {
	return f.history, f.err
}

func (f *fakeChatSource) Profile(_ context.Context, _ int64) (*userbot.UserProfile, error) {
	return f.profile, f.err
}

func TestGetChatHistoryToolSchema(t *testing.T) {
	tool := GetChatHistoryTool()
	if tool.Name != "get_chat_history" {
		t.Fatalf("unexpected name: %s", tool.Name)
	}
	if tool.Description == "" {
		t.Fatal("description is empty")
	}
	if tool.Parameters == nil {
		t.Fatal("parameters schema is missing")
	}
}

func TestExecuteGetChatHistory(t *testing.T) {
	src := &fakeChatSource{
		history: []userbot.Message{{ID: 1, SenderID: 42, Text: "привет"}},
	}

	raw := json.RawMessage(`{"chat_id": 42}`)
	out, err := ExecuteGetChatHistory(context.Background(), src, raw)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var msgs []userbot.Message
	if err := json.Unmarshal([]byte(out), &msgs); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Text != "привет" || msgs[0].SenderID != 42 {
		t.Fatalf("unexpected result: %+v", msgs)
	}
}

func TestExecuteGetChatHistoryInvalidArgs(t *testing.T) {
	src := &fakeChatSource{}

	cases := []string{
		`{}`,
		`{"chat_id": 42, "limit": 0}`,
		`{"chat_id": 42, "limit": 51}`,
		`not json`,
	}
	for _, raw := range cases {
		if _, err := ExecuteGetChatHistory(context.Background(), src, json.RawMessage(raw)); err == nil {
			t.Fatalf("expected error for args %q", raw)
		}
	}
}

func TestExecuteGetChatHistoryLimit(t *testing.T) {
	var gotLimit int
	src := &fakeChatSource{
		history: []userbot.Message{{ID: 1}},
	}
	chat := chatSourceFunc(func(ctx context.Context, chatID int64, limit int) ([]userbot.Message, error) {
		gotLimit = limit
		return src.history, nil
	})

	if _, err := ExecuteGetChatHistory(context.Background(), chat, json.RawMessage(`{"chat_id": 1, "limit": 30}`)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if gotLimit != 30 {
		t.Fatalf("expected limit 30, got %d", gotLimit)
	}
}

func TestExecuteGetChatHistorySourceError(t *testing.T) {
	src := &fakeChatSource{err: errors.New("boom")}
	if _, err := ExecuteGetChatHistory(context.Background(), src, json.RawMessage(`{"chat_id": 1}`)); err == nil {
		t.Fatal("expected error")
	}
}

func TestExecuteGetUserProfile(t *testing.T) {
	src := &fakeChatSource{
		profile: &userbot.UserProfile{ID: 7, FirstName: "Anna", Username: "anna", Bio: "boss"},
	}

	out, err := ExecuteGetUserProfile(context.Background(), src, json.RawMessage(`{"user_id": 7}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var p userbot.UserProfile
	if err := json.Unmarshal([]byte(out), &p); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	if p.ID != 7 || p.FirstName != "Anna" || p.Username != "anna" || p.Bio != "boss" {
		t.Fatalf("unexpected result: %+v", p)
	}
}

func TestExecuteGetUserProfileInvalidArgs(t *testing.T) {
	src := &fakeChatSource{}
	if _, err := ExecuteGetUserProfile(context.Background(), src, json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error for missing user_id")
	}
}

func TestGetUserProfileToolSchema(t *testing.T) {
	tool := GetUserProfileTool()
	if tool.Name != "get_user_profile" {
		t.Fatalf("unexpected name: %s", tool.Name)
	}
}
