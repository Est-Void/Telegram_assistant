package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testCatalog(t *testing.T) *StickerCatalog {
	t.Helper()
	c, err := LoadStickerCatalog(filepath.Join("testdata", "stickers.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return c
}

func TestLoadStickerCatalog(t *testing.T) {
	c := testCatalog(t)

	if len(c.Stickers) != 2 {
		t.Fatalf("stickers = %d, want 2", len(c.Stickers))
	}
	if got, ok := c.Lookup("fire"); !ok || got != "FILE_FIRE" {
		t.Errorf("fire lookup = %q,%v, want FILE_FIRE,true", got, ok)
	}
}

func TestLoadStickerCatalogMissingFile(t *testing.T) {
	if _, err := LoadStickerCatalog(filepath.Join("testdata", "nope.json")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadStickerCatalogMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stickers.json")
	if err := os.WriteFile(path, []byte(`{"stickers":`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadStickerCatalog(path); err == nil {
		t.Fatal("expected error for malformed catalog")
	}
}

func TestCatalogLookupUnknown(t *testing.T) {
	c := testCatalog(t)
	if _, ok := c.Lookup("nope"); ok {
		t.Error("unexpected hit for unknown key")
	}
}

func TestCatalogKeys(t *testing.T) {
	c := testCatalog(t)
	keys := c.Keys()
	if len(keys) != 2 || keys[0] != "fire" || keys[1] != "thumbs_up" {
		t.Errorf("keys = %v, want [fire thumbs_up]", keys)
	}
}

func TestSendStickerToolDescriptor(t *testing.T) {
	c := testCatalog(t)
	tool := SendStickerTool(c)

	if tool.Name != "send_sticker" {
		t.Errorf("unexpected name: %s", tool.Name)
	}
	if tool.Description == "" {
		t.Error("description is empty")
	}
	if !strings.Contains(tool.Description, "fire") {
		t.Errorf("description should list available keys, got: %s", tool.Description)
	}
}

func TestExecuteSendStickerRegular(t *testing.T) {
	c := testCatalog(t)
	fake := &fakeMessenger{}
	deps := Deps{
		ChatID:               12345,
		BusinessConnectionID: "conn-1",
		Messenger:            fake,
	}

	raw := json.RawMessage(`{"sticker":"fire"}`)
	if err := ExecuteSendSticker(context.Background(), deps, c, raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fake.stickerCalls) != 1 {
		t.Fatalf("calls = %d, want 1", len(fake.stickerCalls))
	}
	got := fake.stickerCalls[0]
	if got.ChatID != 12345 {
		t.Errorf("chat_id = %d, want 12345", got.ChatID)
	}
	if got.FileID != "FILE_FIRE" {
		t.Errorf("file_id = %q, want FILE_FIRE", got.FileID)
	}
	if got.ReplyToMessageID != 0 {
		t.Errorf("reply_to_message_id = %d, want 0 (regular message)", got.ReplyToMessageID)
	}
	if got.BusinessConnectionID != "conn-1" {
		t.Errorf("business_connection_id = %q, want conn-1", got.BusinessConnectionID)
	}
}

func TestExecuteSendStickerReply(t *testing.T) {
	c := testCatalog(t)
	fake := &fakeMessenger{}
	deps := Deps{ChatID: 12345, Messenger: fake}

	raw := json.RawMessage(`{"sticker":"thumbs_up","reply_to_message_id":7}`)
	if err := ExecuteSendSticker(context.Background(), deps, c, raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fake.stickerCalls) != 1 {
		t.Fatalf("calls = %d, want 1", len(fake.stickerCalls))
	}
	if got := fake.stickerCalls[0]; got.ReplyToMessageID != 7 {
		t.Errorf("reply_to_message_id = %d, want 7", got.ReplyToMessageID)
	}
}

func TestExecuteSendStickerEmptyKey(t *testing.T) {
	c := testCatalog(t)
	fake := &fakeMessenger{}
	deps := Deps{ChatID: 12345, Messenger: fake}

	raw := json.RawMessage(`{"sticker":""}`)
	if err := ExecuteSendSticker(context.Background(), deps, c, raw); !errors.Is(err, ErrStickerKeyRequired) {
		t.Fatalf("error = %v, want ErrStickerKeyRequired", err)
	}
	if len(fake.stickerCalls) != 0 {
		t.Error("messenger must not be called on invalid args")
	}
}

func TestExecuteSendStickerUnknownKey(t *testing.T) {
	c := testCatalog(t)
	fake := &fakeMessenger{}
	deps := Deps{ChatID: 12345, Messenger: fake}

	raw := json.RawMessage(`{"sticker":"ghost"}`)
	if err := ExecuteSendSticker(context.Background(), deps, c, raw); err == nil {
		t.Fatal("expected error for unknown sticker key")
	}
	if len(fake.stickerCalls) != 0 {
		t.Error("messenger must not be called for unknown key")
	}
}

func TestExecuteSendStickerInvalidReplyID(t *testing.T) {
	c := testCatalog(t)
	fake := &fakeMessenger{}
	deps := Deps{ChatID: 12345, Messenger: fake}

	raw := json.RawMessage(`{"sticker":"fire","reply_to_message_id":0}`)
	if err := ExecuteSendSticker(context.Background(), deps, c, raw); err == nil {
		t.Fatal("expected error for non-positive reply_to_message_id")
	}
	if len(fake.stickerCalls) != 0 {
		t.Error("messenger must not be called on invalid args")
	}
}

func TestExecuteSendStickerMalformedJSON(t *testing.T) {
	c := testCatalog(t)
	fake := &fakeMessenger{}
	deps := Deps{ChatID: 12345, Messenger: fake}

	raw := json.RawMessage(`{"sticker":`)
	if err := ExecuteSendSticker(context.Background(), deps, c, raw); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if len(fake.stickerCalls) != 0 {
		t.Error("messenger must not be called on malformed args")
	}
}

func TestExecuteSendStickerMessengerError(t *testing.T) {
	c := testCatalog(t)
	deps := Deps{ChatID: 12345, Messenger: errorMessenger{}}

	raw := json.RawMessage(`{"sticker":"fire"}`)
	if err := ExecuteSendSticker(context.Background(), deps, c, raw); err == nil {
		t.Fatal("expected error to propagate from messenger")
	}
}
