package application

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/redis/go-redis/v9"

	"github.com/MKmasterg/random-reaction-telegram-bot/internal/config"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/content"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/handler"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/reaction"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/redisstore"
	telegramadapter "github.com/MKmasterg/random-reaction-telegram-bot/internal/telegram"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/transport"
)

type productionRandom struct{}

func (productionRandom) IntN(n int) int   { return rand.IntN(n) }
func (productionRandom) Float64() float64 { return rand.Float64() }

type Runtime struct {
	Bot     *tgbot.Bot
	Handler *handler.Handler
	Redis   *redis.Client
}

func New(cfg config.Config, logger *slog.Logger, options ...tgbot.Option) (*Runtime, error) {
	contentModel, err := content.LoadEmbedded()
	if err != nil {
		return nil, fmt.Errorf("load embedded content: %w", err)
	}
	redisClient, err := redisstore.NewClient(cfg.RedisURL)
	if err != nil {
		return nil, err
	}
	options = append(options, tgbot.WithSkipGetMe())
	client, err := tgbot.New(cfg.Token, options...)
	if err != nil {
		_ = redisClient.Close()
		return nil, fmt.Errorf("initialize Telegram client: %w", err)
	}
	random := productionRandom{}
	application := handler.New(
		telegramadapter.NewDelivery(client),
		reaction.New(contentModel, random),
		redisstore.NewCooldowns(redisClient, cfg.RedisKeyPrefix, cfg.GroupCooldown),
		handler.Options{
			Probability: cfg.ReplyProbability,
			Random:      random,
			Now:         time.Now,
			Logger:      logger,
		},
	)
	return &Runtime{Bot: client, Handler: application, Redis: redisClient}, nil
}

func (r *Runtime) Close() error {
	return r.Redis.Close()
}

func (r *Runtime) Ready(ctx context.Context) error {
	return r.Redis.Ping(ctx).Err()
}

type dispatcher struct {
	handler *handler.Handler
}

func (d dispatcher) Dispatch(ctx context.Context, update *models.Update) error {
	return d.handler.Handle(ctx, telegramadapter.FromUpdate(update))
}

func NewFunctionHandler(cfg config.Config, logger *slog.Logger) (http.Handler, error) {
	runtime, err := New(cfg, logger)
	if err != nil {
		return nil, err
	}
	return transport.NewWebhookHandler(cfg.WebhookSecret, dispatcher{handler: runtime.Handler}, logger), nil
}
