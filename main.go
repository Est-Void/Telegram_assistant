package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"telegram-assistant/internal/agent"
	"telegram-assistant/internal/cache"
	"telegram-assistant/internal/chat"
	"telegram-assistant/internal/config"
	"telegram-assistant/internal/logger"
	"telegram-assistant/internal/tools"
	"telegram-assistant/internal/userbot"
)

func main() {
	authOnly := flag.Bool("auth", false, "authenticate the userbot account and exit")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log, err := logger.New("logs")
	if err != nil {
		slog.Error("init logger", "error", err)
		os.Exit(1)
	}

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Error("load config", "error", err)
		os.Exit(1)
	}

	ub := userbot.New(userbot.Options{
		AppID:      cfg.APIID,
		AppHash:    cfg.APIHash,
		Phone:      cfg.Userbot.Phone,
		SessionDir: cfg.Userbot.SessionDir,
		Logger:     log,
	})

	if *authOnly {
		runAuth(ctx, log, ub)
		return
	}

	store := newCache(ctx, log)

	chats := chat.New(ub, store)

	catalog, err := tools.LoadStickerCatalog("stickers.json")
	if err != nil {
		log.Warn("sticker catalog unavailable, send_sticker disabled", "error", err)
		catalog = nil
	}

	ollamaURL := os.Getenv("OLLAMA_BASE_URL")
	if ollamaURL == "" {
		ollamaURL = cfg.Ollama.BaseURL
	}

	ag := agent.New(agent.Options{
		Model:       cfg.Ollama.Model,
		BaseURL:     ollamaURL,
		Chat:        chats,
		Catalog:     catalog,
		TypingSpeed: cfg.TypingSpeed,
		Logger:      log,
	})
	log.Info("agent configured", "model", cfg.Ollama.Model, "base_url", ollamaURL)

	go func() {
		if err := ub.Run(ctx); err != nil {
			log.Error("userbot stopped", "error", err)
		}
	}()

	if err := waitUserbotReady(ctx, log, ub); err != nil {
		log.Warn("userbot not ready, tools that read history/profile will fail", "error", err)
	}

	b, err := bot.New(cfg.BotToken,
		bot.WithDefaultHandler(businessHandler(log, ag, cfg.AllowedUsers)),
		bot.WithAllowedUpdates(bot.AllowedUpdates{
			models.AllowedUpdateBusinessConnection,
			models.AllowedUpdateBusinessMessage,
			models.AllowedUpdateEditedBusinessMessage,
			models.AllowedUpdateDeletedBusinessMessages,
		}),
	)
	if err != nil {
		log.Error("init bot", "error", err)
		os.Exit(1)
	}

	log.Info("bot started", "bot_id", b.ID())
	b.Start(ctx)
	log.Info("bot stopped")
}

// runAuth connects the userbot, performs interactive authorization (code
// and 2FA password are read from stdin) and saves the session, then exits.
func runAuth(ctx context.Context, log *slog.Logger, ub *userbot.Client) {
	authCtx, authCancel := context.WithTimeout(ctx, userbot.AuthTimeout)
	defer authCancel()

	runCtx, runCancel := context.WithCancel(authCtx)
	defer runCancel()

	errCh := make(chan error, 1)
	go func() { errCh <- ub.Run(runCtx) }()

	select {
	case err := <-errCh:
		if err != nil {
			log.Error("userbot auth failed", "error", err)
			os.Exit(1)
		}
		return
	case <-ub.Ready():
		log.Info("userbot authenticated")
		runCancel()
	}

	if err := <-errCh; err != nil {
		log.Error("userbot shutdown", "error", err)
		os.Exit(1)
	}
}

// newCache connects to Redis; falls back to an in-memory cache if unavailable.
func newCache(ctx context.Context, log *slog.Logger) cache.Cache {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	if r, err := cache.NewRedis(ctx, redisAddr); err == nil {
		log.Info("cache: redis", "addr", redisAddr)
		return r
	} else {
		log.Warn("redis unavailable, using in-memory cache", "addr", redisAddr, "error", err)
	}
	return cache.NewMemory()
}

// waitUserbotReady waits for the userbot connection with a bounded timeout.
func waitUserbotReady(ctx context.Context, log *slog.Logger, ub *userbot.Client) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	select {
	case <-ub.Ready():
	case <-ctx.Done():
		return ctx.Err()
	}

	info, err := ub.Self(ctx)
	if err != nil {
		return err
	}
	log.Info("userbot connected", "user_id", info.ID, "first_name", info.FirstName, "username", info.Username)
	return nil
}

func businessHandler(log *slog.Logger, ag *agent.Agent, allowedUsers map[string]bool) bot.HandlerFunc {
	var b *bot.Bot

	buf := agent.NewBuffer(agent.BufferOptions{
		Window:  5 * time.Second,
		MaxSize: 10,
		Log:     log,
		Flush: func(msgs []*models.Message) {
			if b == nil {
				return
			}
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				ag.HandleChunk(ctx, b, msgs)
			}()
		},
	})

	return func(ctx context.Context, got *bot.Bot, update *models.Update) {
		b = got
		switch {
		case update.BusinessConnection != nil:
			bc := update.BusinessConnection
			log.Info("business connection",
				"connection_id", bc.ID,
				"user_id", bc.User.ID,
				"username", bc.User.Username,
				"is_enabled", bc.IsEnabled,
			)
		case update.BusinessMessage != nil:
			msg := update.BusinessMessage
			if !allowedUsers[strconv.FormatInt(msg.From.ID, 10)] {
				log.Info("ignoring message from non-allowed user",
					"user_id", msg.From.ID,
					"chat_id", msg.Chat.ID,
				)
				break
			}
			buf.Add(msg.Chat.ID, msg)
		case update.EditedBusinessMessage != nil:
			msg := update.EditedBusinessMessage
			log.Info("business message edited",
				"connection_id", msg.BusinessConnectionID,
				"chat_id", msg.Chat.ID,
				"message_id", msg.ID,
				"text", msg.Text,
			)
		case update.DeletedBusinessMessages != nil:
			d := update.DeletedBusinessMessages
			log.Info("business messages deleted",
				"connection_id", d.BusinessConnectionID,
				"chat_id", d.Chat.ID,
				"message_ids", d.MessageIDs,
			)
		}
	}
}
