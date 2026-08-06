package agent

import (
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/go-telegram/bot/models"
)

// Buffer accumulates incoming business messages per chat and delivers them to
// the agent in chunks: a chunk is flushed when no new message arrives within
// the window, or when it reaches maxSize messages.
type Buffer struct {
	log     *slog.Logger
	window  time.Duration
	maxSize int
	now     func() time.Time
	flush   func(msgs []*models.Message)

	mu    sync.Mutex
	chats map[int64]*chatChunk
}

// BufferOptions configures a Buffer.
type BufferOptions struct {
	// Window is the quiet period that ends a chunk.
	Window time.Duration
	// MaxSize flushes a chunk immediately when it grows to this many messages.
	MaxSize int
	// Log receives buffer logs.
	Log *slog.Logger
	// Flush is called with a completed chunk.
	Flush func(msgs []*models.Message)
}

type chatChunk struct {
	msgs   []*models.Message
	timer  *time.Timer
	lastAt time.Time
}

// NewBuffer creates a Buffer.
func NewBuffer(opts BufferOptions) *Buffer {
	log := opts.Log
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Buffer{
		log:     log,
		window:  opts.Window,
		maxSize: opts.MaxSize,
		now:     time.Now,
		flush:   opts.Flush,
		chats:   make(map[int64]*chatChunk),
	}
}

// Add queues a message into the current chunk of its chat, flushing the
// previous chunk if the gap exceeded the window.
func (b *Buffer) Add(chatID int64, msg *models.Message) {
	b.mu.Lock()

	st, ok := b.chats[chatID]
	now := b.now()

	if ok && now.Sub(st.lastAt) > b.window {
		msgs := st.take()
		b.mu.Unlock()
		b.deliver(chatID, msgs)
		b.mu.Lock()
		st, ok = b.chats[chatID]
	}

	if !ok {
		st = &chatChunk{}
		b.chats[chatID] = st
	}

	st.msgs = append(st.msgs, msg)
	st.lastAt = now

	if len(st.msgs) >= b.maxSize {
		msgs := st.take()
		b.mu.Unlock()
		b.deliver(chatID, msgs)
		return
	}

	if st.timer != nil {
		st.timer.Stop()
	}
	st.timer = time.AfterFunc(b.window, func() { b.flushChat(chatID) })
	b.mu.Unlock()
}

// flushChat fires when a chat's chunk sat quietly for the window.
func (b *Buffer) flushChat(chatID int64) {
	b.mu.Lock()
	st, ok := b.chats[chatID]
	if !ok {
		b.mu.Unlock()
		return
	}
	msgs := st.take()
	b.mu.Unlock()
	b.deliver(chatID, msgs)
}

func (b *Buffer) deliver(chatID int64, msgs []*models.Message) {
	if len(msgs) == 0 {
		return
	}
	b.log.Info("agent: chunk flushed", "chat_id", chatID, "count", len(msgs))
	if b.flush != nil {
		b.flush(msgs)
	}
}

func (st *chatChunk) take() []*models.Message {
	msgs := st.msgs
	st.msgs = nil
	if st.timer != nil {
		st.timer.Stop()
		st.timer = nil
	}
	return msgs
}
