package chat

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"telegram-assistant/internal/cache"
	"telegram-assistant/internal/userbot"
)

type fakeSource struct {
	mux       sync.Mutex
	histCalls int
	profCalls int
	history   []userbot.Message
	profile   *userbot.UserProfile
	histErr   error
	profErr   error
}

func (f *fakeSource) History(_ context.Context, _ int64, _ int) ([]userbot.Message, error) {
	f.mux.Lock()
	defer f.mux.Unlock()
	f.histCalls++
	return f.history, f.histErr
}

func (f *fakeSource) Profile(_ context.Context, _ int64) (*userbot.UserProfile, error) {
	f.mux.Lock()
	defer f.mux.Unlock()
	f.profCalls++
	return f.profile, f.profErr
}

func TestHistoryCached(t *testing.T) {
	src := &fakeSource{
		history: []userbot.Message{{ID: 1, Text: "hi"}},
	}
	svc := New(src, cache.NewMemory())
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		msgs, err := svc.History(ctx, 42, 10)
		if err != nil {
			t.Fatalf("history: %v", err)
		}
		if len(msgs) != 1 || msgs[0].Text != "hi" {
			t.Fatalf("unexpected history: %+v", msgs)
		}
	}
	if src.histCalls != 1 {
		t.Fatalf("expected 1 userbot call, got %d", src.histCalls)
	}
}

func TestHistoryErrorNotCached(t *testing.T) {
	src := &fakeSource{histErr: errors.New("boom")}
	svc := New(src, cache.NewMemory())
	ctx := context.Background()

	if _, err := svc.History(ctx, 42, 10); err == nil {
		t.Fatal("expected error")
	}
	src.mux.Lock()
	src.histErr = nil
	src.mux.Unlock()

	if _, err := svc.History(ctx, 42, 10); err != nil {
		t.Fatalf("second call should succeed, got %v", err)
	}
}

func TestHistorySingleflight(t *testing.T) {
	src := &fakeSource{
		history: []userbot.Message{{ID: 1, Text: "hi"}},
	}
	svc := New(src, cache.NewMemory())
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.History(ctx, 7, 5); err != nil {
				t.Errorf("history: %v", err)
			}
		}()
	}
	wg.Wait()

	if src.histCalls != 1 {
		t.Fatalf("expected 1 userbot call under singleflight, got %d", src.histCalls)
	}
}

func TestProfileCached(t *testing.T) {
	src := &fakeSource{
		profile: &userbot.UserProfile{ID: 1, FirstName: "Anna", Bio: "boss"},
	}
	svc := New(src, cache.NewMemory())
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		p, err := svc.Profile(ctx, 1)
		if err != nil {
			t.Fatalf("profile: %v", err)
		}
		if p.FirstName != "Anna" || p.Bio != "boss" {
			t.Fatalf("unexpected profile: %+v", p)
		}
	}
	if src.profCalls != 1 {
		t.Fatalf("expected 1 userbot call, got %d", src.profCalls)
	}
}

func TestFrame(t *testing.T) {
	src := &fakeSource{
		history: []userbot.Message{
			{ID: 1, SenderID: 1, SenderName: "Ivan", Date: time.Now().Unix(), Text: "привет"},
		},
	}
	svc := New(src, cache.NewMemory())

	frame, err := svc.Frame(context.Background(), 42, 10)
	if err != nil {
		t.Fatalf("frame: %v", err)
	}
	if frame == "" {
		t.Fatal("expected non-empty frame")
	}
}
