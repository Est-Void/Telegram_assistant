package userbot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/contrib/middleware/ratelimit"
	"github.com/gotd/log/logslog"
	"go.etcd.io/bbolt"
	"golang.org/x/time/rate"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
)

const (
	dialogsBucket = "dialogs"
	phoneBucket   = "phones"
	stateBucket   = "state"
	maxBootstrap  = 200
)

// Message is a trimmed representation of a Telegram message.
type Message struct {
	ID         int    `json:"id"`
	SenderID   int64  `json:"sender_id"`
	SenderName string `json:"sender_name"`
	Date       int64  `json:"date"`
	Text       string `json:"text"`
	Out        bool   `json:"out"`
}

// UserProfile is a trimmed user profile.
type UserProfile struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	Bio       string `json:"bio"`
}

// SelfInfo is a trimmed view of the authenticated account.
type SelfInfo struct {
	ID        int64
	FirstName string
	Username  string
}

// Options configures the userbot client.
type Options struct {
	AppID      int64
	AppHash    string
	Phone      string
	SessionDir string
	Logger     *slog.Logger
}

// Client is a userbot for the business account.
//
// It logs into the account via MTProto, keeps the connection alive and
// provides read-only access to chat history and user profiles.
type Client struct {
	opts Options
	log  *slog.Logger

	client     *telegram.Client
	manager    *peers.Manager
	gaps       *updates.Manager
	updateHook telegram.UpdateHandler
	db         *bbolt.DB

	ready chan struct{}
}

// New creates a userbot client.
func New(opts Options) *Client {
	return &Client{
		opts:  opts,
		log:   opts.Logger,
		ready: make(chan struct{}),
	}
}

// Ready returns a channel closed once the client is authenticated and
// peer storage is bootstrapped.
func (c *Client) Ready() <-chan struct{} {
	return c.ready
}

// Run connects to Telegram and serves read requests until ctx is cancelled.
func (c *Client) Run(ctx context.Context) error {
	if err := os.MkdirAll(c.opts.SessionDir, 0o755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	db, err := bbolt.Open(filepath.Join(c.opts.SessionDir, "userbot.db"), 0o644, &bbolt.Options{
		NoSync:         true,
		NoFreelistSync: true,
	})
	if err != nil {
		return fmt.Errorf("open userbot db: %w", err)
	}
	c.db = db
	defer func() { _ = db.Close() }()

	if err := db.Update(func(tx *bbolt.Tx) error {
		for _, name := range []string{dialogsBucket, phoneBucket, stateBucket} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("init userbot db: %w", err)
	}

	logger := c.log
	waiter := floodwait.NewWaiter().WithCallback(func(ctx context.Context, wait floodwait.FloodWait) {
		logger.Warn("flood wait", "duration", wait.Duration)
	})

	peerStorage := &peerStorage{db: db}
	var hook telegram.UpdateHandler

	client := telegram.NewClient(int(c.opts.AppID), c.opts.AppHash, telegram.Options{
		Logger: logslog.New(logger),
		SessionStorage: &session.FileStorage{
			Path: filepath.Join(c.opts.SessionDir, "userbot.session"),
		},
		Middlewares: []telegram.Middleware{
			waiter,
			ratelimit.New(rate.Every(5*time.Second), 1),
		},
		UpdateHandler: telegram.UpdateHandlerFunc(func(ctx context.Context, u tg.UpdatesClass) error {
			return hook.Handle(ctx, u)
		}),
	})
	c.client = client

	manager := peers.Options{
		Storage: peerStorage,
		Logger:  logslog.New(logger),
	}.Build(client.API())
	c.manager = manager

	gaps := updates.New(updates.Config{
		Handler:      tg.NewUpdateDispatcher(),
		AccessHasher: manager,
		Logger:       logslog.New(logger),
	})
	c.gaps = gaps
	hook = manager.UpdateHook(gaps)

	return waiter.Run(ctx, func(ctx context.Context) error {
		return client.Run(ctx, func(ctx context.Context) error {
			if err := c.auth(ctx); err != nil {
				return fmt.Errorf("auth: %w", err)
			}

			if err := manager.Init(ctx); err != nil {
				return fmt.Errorf("init peers: %w", err)
			}

			self, err := manager.Self(ctx)
			if err != nil {
				return fmt.Errorf("get self: %w", err)
			}

			if err := c.bootstrap(ctx); err != nil {
				return fmt.Errorf("bootstrap peers: %w", err)
			}
			c.log.Info("peer storage bootstrapped from dialogs")

			c.markReady()

			_, isBot := self.ToBot()
			if err := gaps.Run(ctx, client.API(), self.ID(), updates.AuthOptions{
				IsBot: isBot,
			}); err != nil {
				return fmt.Errorf("updates: %w", err)
			}

			<-ctx.Done()
			return nil
		})
	})
}

func (c *Client) markReady() {
	select {
	case <-c.ready:
	default:
		close(c.ready)
	}
}

// bootstrap fills peer storage with access hashes from recent dialogs.
func (c *Client) bootstrap(ctx context.Context) error {
	iter := dialogs.NewQueryBuilder(c.client.API()).
		GetDialogs().
		BatchSize(100).
		Iter()

	count := 0
	c.log.Debug("bootstrap: fetching dialogs")
	for iter.Next(ctx) {
		e := iter.Value()
		users := make([]tg.UserClass, 0, len(e.Entities.Users()))
		for _, u := range e.Entities.Users() {
			users = append(users, u)
		}
		chats := make([]tg.ChatClass, 0, len(e.Entities.Chats())+len(e.Entities.Channels()))
		for _, ch := range e.Entities.Chats() {
			chats = append(chats, ch)
		}
		for _, ch := range e.Entities.Channels() {
			chats = append(chats, ch)
		}
		if err := c.manager.Apply(ctx, users, chats); err != nil {
			return err
		}
		count++
		if count >= maxBootstrap {
			break
		}
	}
	return iter.Err()
}

// History returns the most recent messages of a chat, newest first.
func (c *Client) History(ctx context.Context, chatID int64, limit int) ([]Message, error) {
	if err := c.waitReady(ctx); err != nil {
		return nil, err
	}

	peer, err := c.resolvePeer(ctx, chatID)
	if err != nil {
		return nil, err
	}

	iter := messages.NewQueryBuilder(c.client.API()).
		GetHistory(peer.InputPeer()).
		BatchSize(limit).
		Iter()

	var msgs []Message
	for iter.Next(ctx) {
		msgs = append(msgs, c.toMessage(ctx, iter.Value().Msg))
		if len(msgs) >= limit {
			break
		}
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return msgs, nil
}

// Profile returns profile information about a user.
func (c *Client) Profile(ctx context.Context, userID int64) (*UserProfile, error) {
	if err := c.waitReady(ctx); err != nil {
		return nil, err
	}

	peer, err := c.resolvePeer(ctx, userID)
	if err != nil {
		return nil, err
	}

	user, ok := peer.(peers.User)
	if !ok {
		return nil, fmt.Errorf("peer %d is not a user", userID)
	}

	raw := user.Raw()
	p := &UserProfile{
		ID:        raw.ID,
		FirstName: raw.FirstName,
		LastName:  raw.LastName,
	}
	if username, ok := raw.GetUsername(); ok {
		p.Username = username
	}

	full, err := c.client.API().UsersGetFullUser(ctx, &tg.InputUser{
		UserID:     raw.ID,
		AccessHash: raw.AccessHash,
	})
	if err != nil {
		c.log.Warn("get full user", "error", err, "user_id", raw.ID)
		return p, nil
	}
	p.Bio = full.FullUser.About
	return p, nil
}

// Self returns info about the authenticated account.
func (c *Client) Self(ctx context.Context) (*SelfInfo, error) {
	if err := c.waitReady(ctx); err != nil {
		return nil, err
	}

	u, err := c.manager.Self(ctx)
	if err != nil {
		return nil, err
	}
	raw := u.Raw()
	info := &SelfInfo{ID: raw.ID, FirstName: raw.FirstName}
	if username, ok := raw.GetUsername(); ok {
		info.Username = username
	}
	return info, nil
}

func (c *Client) waitReady(ctx context.Context) error {
	select {
	case <-c.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) resolvePeer(ctx context.Context, chatID int64) (peers.Peer, error) {
	var err error
	if chatID > 0 {
		u, e := c.manager.ResolveUserID(ctx, chatID)
		if e == nil {
			return u, nil
		}
		err = e
	} else {
		p, e := c.manager.ResolveChatID(ctx, chatID)
		if e == nil {
			return p, nil
		}
		err = e
		if p, e := c.manager.ResolveChannelID(ctx, chatID); e == nil {
			return p, nil
		}
	}
	return nil, fmt.Errorf("resolve peer %d: %w (add it to a recent dialog first)", chatID, err)
}

func (c *Client) toMessage(ctx context.Context, m tg.NotEmptyMessage) Message {
	msg := Message{
		ID:   m.GetID(),
		Date: int64(m.GetDate()),
		Out:  m.GetOut(),
	}
	if full, ok := m.(*tg.Message); ok {
		msg.Text = full.Message
	}
	if from, ok := m.GetFromID(); ok && from != nil {
		msg.SenderID = peerID(from)
		if peer, err := c.manager.ResolvePeer(ctx, from); err == nil {
			msg.SenderName = peer.VisibleName()
		}
	}
	return msg
}

func peerID(p tg.PeerClass) int64 {
	switch p := p.(type) {
	case *tg.PeerUser:
		return p.UserID
	case *tg.PeerChat:
		return p.ChatID
	case *tg.PeerChannel:
		return p.ChannelID
	default:
		return 0
	}
}
