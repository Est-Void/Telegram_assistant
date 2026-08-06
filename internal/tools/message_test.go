package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestSendMessageToolDescriptor(t *testing.T) {
	tool := SendMessageTool()

	if tool.Name != "send_message" {
		t.Errorf("unexpected name: %s", tool.Name)
	}
	if tool.Description == "" {
		t.Error("description is empty")
	}

	schema, ok := tool.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("parameters is not a schema object: %T", tool.Parameters)
	}
	if schema["type"] != "object" {
		t.Errorf("schema type = %v, want object", schema["type"])
	}
}

func TestExecuteSendMessageRegular(t *testing.T) {
	fake := &fakeMessenger{}
	deps := Deps{
		ChatID:               12345,
		BusinessConnectionID: "conn-1",
		Messenger:            fake,
	}

	raw := json.RawMessage(`{"text":"hello"}`)
	if err := ExecuteSendMessage(context.Background(), deps, raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fake.messageCalls) != 1 {
		t.Fatalf("calls = %d, want 1", len(fake.messageCalls))
	}
	got := fake.messageCalls[0]
	if got.ChatID != 12345 {
		t.Errorf("chat_id = %d, want 12345", got.ChatID)
	}
	if got.Text != "hello" {
		t.Errorf("text = %q, want %q", got.Text, "hello")
	}
	if got.ReplyToMessageID != 0 {
		t.Errorf("reply_to_message_id = %d, want 0 (regular message)", got.ReplyToMessageID)
	}
	if got.BusinessConnectionID != "conn-1" {
		t.Errorf("business_connection_id = %q, want conn-1", got.BusinessConnectionID)
	}
}

func TestExecuteSendMessageReply(t *testing.T) {
	fake := &fakeMessenger{}
	deps := Deps{ChatID: 12345, Messenger: fake}

	raw := json.RawMessage(`{"text":"reply","reply_to_message_id":42}`)
	if err := ExecuteSendMessage(context.Background(), deps, raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fake.messageCalls) != 1 {
		t.Fatalf("calls = %d, want 1", len(fake.messageCalls))
	}
	if got := fake.messageCalls[0]; got.ReplyToMessageID != 42 {
		t.Errorf("reply_to_message_id = %d, want 42", got.ReplyToMessageID)
	}
}

func TestExecuteSendMessageEmptyText(t *testing.T) {
	fake := &fakeMessenger{}
	deps := Deps{ChatID: 12345, Messenger: fake}

	raw := json.RawMessage(`{"text":""}`)
	if err := ExecuteSendMessage(context.Background(), deps, raw); !errors.Is(err, ErrTextRequired) {
		t.Fatalf("error = %v, want ErrTextRequired", err)
	}
	if len(fake.messageCalls) != 0 {
		t.Error("messenger must not be called on invalid args")
	}
}

func TestExecuteSendMessageInvalidReplyID(t *testing.T) {
	fake := &fakeMessenger{}
	deps := Deps{ChatID: 12345, Messenger: fake}

	raw := json.RawMessage(`{"text":"x","reply_to_message_id":0}`)
	if err := ExecuteSendMessage(context.Background(), deps, raw); err == nil {
		t.Fatal("expected error for non-positive reply_to_message_id")
	}
	if len(fake.messageCalls) != 0 {
		t.Error("messenger must not be called on invalid args")
	}
}

func TestExecuteSendMessageMalformedJSON(t *testing.T) {
	fake := &fakeMessenger{}
	deps := Deps{ChatID: 12345, Messenger: fake}

	raw := json.RawMessage(`{"text":`)
	if err := ExecuteSendMessage(context.Background(), deps, raw); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if len(fake.messageCalls) != 0 {
		t.Error("messenger must not be called on malformed args")
	}
}

func TestExecuteSendMessageMessengerError(t *testing.T) {
	deps := Deps{ChatID: 12345, Messenger: errorMessenger{}}

	raw := json.RawMessage(`{"text":"x"}`)
	if err := ExecuteSendMessage(context.Background(), deps, raw); err == nil {
		t.Fatal("expected error to propagate from messenger")
	}
}
