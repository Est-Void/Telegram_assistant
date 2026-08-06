package agent

import (
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
)

func testMsg(id int, chatID int64) *models.Message {
	return &models.Message{
		ID:   id,
		Chat: models.Chat{ID: chatID},
		From: &models.User{ID: 8073087570},
		Text: "msg",
	}
}

func waitFlush(t *testing.T, ch <-chan []*models.Message) []*models.Message {
	t.Helper()
	select {
	case msgs := <-ch:
		return msgs
	case <-time.After(2 * time.Second):
		t.Fatal("expected chunk flush, got none")
		return nil
	}
}

func ids(msgs []*models.Message) []int {
	out := make([]int, len(msgs))
	for i, m := range msgs {
		out[i] = m.ID
	}
	return out
}

func eqInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBufferFlushesAtMaxSize(t *testing.T) {
	flushed := make(chan []*models.Message, 10)
	buf := NewBuffer(BufferOptions{
		Window:  time.Hour,
		MaxSize: 2,
		Flush:   func(msgs []*models.Message) { flushed <- msgs },
	})
	now := time.Now()
	buf.now = func() time.Time { return now }

	buf.Add(100, testMsg(1, 100))
	select {
	case <-flushed:
		t.Fatal("chunk flushed before max size reached")
	default:
	}

	buf.Add(100, testMsg(2, 100))
	got := waitFlush(t, flushed)
	if !eqInts(ids(got), []int{1, 2}) {
		t.Errorf("flushed ids = %v, want [1 2]", ids(got))
	}
}

func TestBufferFlushesOnGap(t *testing.T) {
	flushed := make(chan []*models.Message, 10)
	buf := NewBuffer(BufferOptions{
		Window:  time.Hour,
		MaxSize: 10,
		Flush:   func(msgs []*models.Message) { flushed <- msgs },
	})
	now := time.Now()
	buf.now = func() time.Time { return now }

	buf.Add(100, testMsg(1, 100))
	buf.Add(100, testMsg(2, 100))

	now = now.Add(2 * time.Hour)
	buf.Add(100, testMsg(3, 100))

	got := waitFlush(t, flushed)
	if !eqInts(ids(got), []int{1, 2}) {
		t.Errorf("flushed ids = %v, want [1 2]", ids(got))
	}
}

func TestBufferFlushesAfterWindow(t *testing.T) {
	flushed := make(chan []*models.Message, 10)
	buf := NewBuffer(BufferOptions{
		Window:  50 * time.Millisecond,
		MaxSize: 10,
		Flush:   func(msgs []*models.Message) { flushed <- msgs },
	})

	buf.Add(100, testMsg(1, 100))
	buf.Add(100, testMsg(2, 100))

	got := waitFlush(t, flushed)
	if !eqInts(ids(got), []int{1, 2}) {
		t.Errorf("flushed ids = %v, want [1 2]", ids(got))
	}
}

func TestBufferChatsAreIsolated(t *testing.T) {
	flushed := make(chan []*models.Message, 10)
	buf := NewBuffer(BufferOptions{
		Window:  time.Hour,
		MaxSize: 10,
		Flush:   func(msgs []*models.Message) { flushed <- msgs },
	})
	now := time.Now()
	buf.now = func() time.Time { return now }

	buf.Add(100, testMsg(1, 100))
	buf.Add(200, testMsg(10, 200))

	now = now.Add(2 * time.Hour)
	buf.Add(100, testMsg(2, 100))

	got := waitFlush(t, flushed)
	if !eqInts(ids(got), []int{1}) {
		t.Errorf("flushed ids = %v, want [1]", ids(got))
	}

	now = now.Add(2 * time.Hour)
	buf.Add(200, testMsg(11, 200))

	got = waitFlush(t, flushed)
	if !eqInts(ids(got), []int{10}) {
		t.Errorf("flushed ids = %v, want [10]", ids(got))
	}
}
