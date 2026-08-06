package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type recordingScheduler struct {
	delay time.Duration
	task  func(ctx context.Context)
	calls int
}

func (r *recordingScheduler) schedule(ctx context.Context, delay time.Duration, task func(ctx context.Context)) error {
	r.calls++
	r.delay = delay
	r.task = task
	return nil
}

func TestExecuteLaterDelayWithMessages(t *testing.T) {
	fake := &fakeMessenger{}
	sched := &recordingScheduler{}
	slept := time.Duration(0)
	sleep := func(_ context.Context, d time.Duration) error {
		slept += d
		return nil
	}

	deps := Deps{
		ChatID:      12345,
		Messenger:   fake,
		TypingSpeed: 10,
		Sleep:       sleep,
		Schedule:    sched.schedule,
	}

	raw := json.RawMessage(`{"in_seconds":5,"messages":[{"text":"first"},{"text":"second"}]}`)
	if err := ExecuteLater(context.Background(), deps, raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sched.calls != 1 {
		t.Fatalf("schedule calls = %d, want 1", sched.calls)
	}
	if sched.delay != 5*time.Second {
		t.Errorf("delay = %v, want 5s", sched.delay)
	}
	if sched.task == nil {
		t.Fatal("task is nil")
	}

	sched.task(context.Background())
	if len(fake.messageCalls) != 2 {
		t.Fatalf("messenger calls = %d, want 2", len(fake.messageCalls))
	}
	if got := fake.messageCalls[1].Text; got != "second" {
		t.Errorf("second message text = %q, want %q", got, "second")
	}
	if want := TypingDelay("first", 10) + TypingDelay("second", 10); slept != want {
		t.Errorf("total typing delay = %v, want %v", slept, want)
	}
}

func TestExecuteLaterAtAbsoluteTime(t *testing.T) {
	fake := &fakeMessenger{}
	sched := &recordingScheduler{}
	deps := Deps{ChatID: 12345, Messenger: fake, Schedule: sched.schedule}

	at := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	raw, _ := json.Marshal(map[string]any{"at": at, "messages": []SendMessageArgs{{Text: "later"}}})

	if err := ExecuteLater(context.Background(), deps, raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sched.delay <= 0 || sched.delay > 2*time.Hour {
		t.Errorf("delay = %v, want between 0 and 2h", sched.delay)
	}
	if sched.task == nil {
		t.Fatal("task is nil")
	}
	sched.task(context.Background())
	if len(fake.messageCalls) != 1 {
		t.Errorf("messenger calls = %d, want 1", len(fake.messageCalls))
	}
}

func TestExecuteLaterCallAgent(t *testing.T) {
	fake := &fakeMessenger{}
	sched := &recordingScheduler{}
	reinvoked := 0
	reinvoke := func(ctx context.Context) error {
		reinvoked++
		return nil
	}

	deps := Deps{
		ChatID:        12345,
		Messenger:     fake,
		Schedule:      sched.schedule,
		ReinvokeAgent: reinvoke,
	}

	raw := json.RawMessage(`{"in_seconds":10,"call_agent":true}`)
	if err := ExecuteLater(context.Background(), deps, raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sched.task(context.Background())
	if reinvoked != 1 {
		t.Errorf("reinvoke calls = %d, want 1", reinvoked)
	}
}

func TestExecuteLaterCallAgentNotConfigured(t *testing.T) {
	deps := Deps{ChatID: 12345}
	raw := json.RawMessage(`{"in_seconds":10,"call_agent":true}`)
	if err := ExecuteLater(context.Background(), deps, raw); err == nil {
		t.Fatal("expected error when ReinvokeAgent is not configured")
	}
}

func TestExecuteLaterBothTimes(t *testing.T) {
	deps := Deps{ChatID: 12345}
	raw := json.RawMessage(`{"in_seconds":5,"at":"2026-08-06T15:04:05+02:00","messages":[{"text":"x"}]}`)
	if err := ExecuteLater(context.Background(), deps, raw); err == nil {
		t.Fatal("expected error when both in_seconds and at are set")
	}
}

func TestExecuteLaterNoTime(t *testing.T) {
	deps := Deps{ChatID: 12345}
	raw := json.RawMessage(`{"messages":[{"text":"x"}]}`)
	if err := ExecuteLater(context.Background(), deps, raw); err == nil {
		t.Fatal("expected error when neither in_seconds nor at is set")
	}
}

func TestExecuteLaterNegativeDelay(t *testing.T) {
	deps := Deps{ChatID: 12345}
	raw := json.RawMessage(`{"in_seconds":-5,"messages":[{"text":"x"}]}`)
	if err := ExecuteLater(context.Background(), deps, raw); err == nil {
		t.Fatal("expected error for negative in_seconds")
	}
}

func TestExecuteLaterAtInPast(t *testing.T) {
	deps := Deps{ChatID: 12345}
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	raw, _ := json.Marshal(map[string]any{"at": past, "messages": []SendMessageArgs{{Text: "x"}}})
	if err := ExecuteLater(context.Background(), deps, raw); err == nil {
		t.Fatal("expected error for at time in the past")
	}
}

func TestExecuteLaterNoAction(t *testing.T) {
	deps := Deps{ChatID: 12345}
	raw := json.RawMessage(`{"in_seconds":5}`)
	if err := ExecuteLater(context.Background(), deps, raw); err == nil {
		t.Fatal("expected error when no action is set")
	}
}

func TestExecuteLaterMessageWithEmptyText(t *testing.T) {
	deps := Deps{ChatID: 12345}
	raw := json.RawMessage(`{"in_seconds":5,"messages":[{"text":""}]}`)
	if err := ExecuteLater(context.Background(), deps, raw); err == nil {
		t.Fatal("expected error for message with empty text")
	}
}

func TestExecuteLaterMalformedJSON(t *testing.T) {
	deps := Deps{ChatID: 12345}
	raw := json.RawMessage(`{"in_seconds":`)
	if err := ExecuteLater(context.Background(), deps, raw); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestExecuteLaterScheduleError(t *testing.T) {
	sched := func(context.Context, time.Duration, func(context.Context)) error {
		return context.DeadlineExceeded
	}
	deps := Deps{ChatID: 12345, Schedule: sched}
	raw := json.RawMessage(`{"in_seconds":5,"messages":[{"text":"x"}]}`)
	if err := ExecuteLater(context.Background(), deps, raw); err == nil {
		t.Fatal("expected error to propagate from scheduler")
	}
}

func TestExecuteLaterTaskMessagesWithReply(t *testing.T) {
	fake := &fakeMessenger{}
	sched := &recordingScheduler{}
	deps := Deps{ChatID: 12345, Messenger: fake, Schedule: sched.schedule}

	raw := json.RawMessage(`{"in_seconds":5,"messages":[{"text":"reply this","reply_to_message_id":9}]}`)
	if err := ExecuteLater(context.Background(), deps, raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sched.task(context.Background())
	if len(fake.messageCalls) != 1 {
		t.Fatalf("messenger calls = %d, want 1", len(fake.messageCalls))
	}
	if got := fake.messageCalls[0].ReplyToMessageID; got != 9 {
		t.Errorf("reply_to_message_id = %d, want 9", got)
	}
}
