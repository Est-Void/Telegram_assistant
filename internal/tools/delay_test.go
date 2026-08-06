package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestTypingDelay(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		speed float64
		want  time.Duration
	}{
		{"ascii", "hello", 5, time.Second},
		{"empty text", "", 5, 0},
		{"zero speed", "hello", 0, 0},
		{"negative speed", "hello", -1, 0},
		{"cyrillic counts runes not bytes", "привет", 6, time.Second},
		{"non-integer speed", "hello", 2.5, 2 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TypingDelay(tt.text, tt.speed); got != tt.want {
				t.Errorf("TypingDelay(%q, %v) = %v, want %v", tt.text, tt.speed, got, tt.want)
			}
		})
	}
}

func TestExecuteSendMessageWaitsTypingDelay(t *testing.T) {
	fake := &fakeMessenger{}
	var slept time.Duration
	sleep := func(_ context.Context, d time.Duration) error {
		slept += d
		return nil
	}

	deps := Deps{
		ChatID:      12345,
		Messenger:   fake,
		TypingSpeed: 5,
		Sleep:       sleep,
	}

	raw := json.RawMessage(`{"text":"hello"}`)
	if err := ExecuteSendMessage(context.Background(), deps, raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := TypingDelay("hello", 5); slept != want {
		t.Errorf("slept = %v, want %v", slept, want)
	}
	if len(fake.messageCalls) != 1 {
		t.Errorf("messenger calls = %d, want 1", len(fake.messageCalls))
	}
}

func TestExecuteSendMessageNoDelayWhenSpeedZero(t *testing.T) {
	fake := &fakeMessenger{}
	sleepCalls := 0
	sleep := func(_ context.Context, d time.Duration) error {
		sleepCalls++
		return nil
	}

	deps := Deps{ChatID: 12345, Messenger: fake, Sleep: sleep}

	raw := json.RawMessage(`{"text":"hello"}`)
	if err := ExecuteSendMessage(context.Background(), deps, raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sleepCalls != 1 {
		t.Errorf("sleep calls = %d, want 1 (with zero duration)", sleepCalls)
	}
	if len(fake.messageCalls) != 1 {
		t.Errorf("messenger calls = %d, want 1", len(fake.messageCalls))
	}
}

func TestExecuteSendMessageSleepError(t *testing.T) {
	fake := &fakeMessenger{}
	sleep := func(context.Context, time.Duration) error {
		return context.Canceled
	}

	deps := Deps{ChatID: 12345, Messenger: fake, TypingSpeed: 5, Sleep: sleep}

	raw := json.RawMessage(`{"text":"hello"}`)
	if err := ExecuteSendMessage(context.Background(), deps, raw); err == nil {
		t.Fatal("expected error when sleep fails")
	}
	if len(fake.messageCalls) != 0 {
		t.Error("messenger must not be called when sleep fails")
	}
}
