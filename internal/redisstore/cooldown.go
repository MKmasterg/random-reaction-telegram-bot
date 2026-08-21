package redisstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/MKmasterg/random-reaction-telegram-bot/internal/handler"
)

const (
	reservationTTL      = 30 * time.Second
	redisOperationLimit = time.Second
)

var reserveScript = redis.NewScript(`local current = redis.call("GET", KEYS[1])
if not current then
  redis.call("SET", KEYS[1], ARGV[2], "PX", ARGV[3])
  return 1
end
if string.sub(current, 1, string.len(ARGV[1])) == ARGV[1] then return 2 end
return 0`)

var finishScript = redis.NewScript(`if redis.call("GET", KEYS[1]) ~= ARGV[1] then return 0 end
if ARGV[2] == "1" then
  redis.call("SET", KEYS[1], "cooldown", "PX", ARGV[3])
  return 1
end
return redis.call("DEL", KEYS[1])`)

type scriptExecutor interface {
	Run(ctx context.Context, script *redis.Script, keys []string, args ...any) (int64, error)
}

type redisExecutor struct {
	client redis.Scripter
}

func (e redisExecutor) Run(ctx context.Context, script *redis.Script, keys []string, args ...any) (int64, error) {
	return script.Run(ctx, e.client, keys, args...).Int64()
}

type Cooldowns struct {
	executor scriptExecutor
	prefix   string
	duration time.Duration
}

func NewCooldowns(client redis.Scripter, prefix string, duration time.Duration) *Cooldowns {
	return &Cooldowns{executor: redisExecutor{client: client}, prefix: prefix, duration: duration}
}

func NewClient(rawURL string) (*redis.Client, error) {
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, errors.New("parse REDIS_URL")
	}
	options.DialTimeout = redisOperationLimit
	options.ReadTimeout = redisOperationLimit
	options.WriteTimeout = redisOperationLimit
	options.PoolTimeout = redisOperationLimit
	options.PoolSize = 4
	options.MinIdleConns = 0
	options.MaxRetries = 1
	return redis.NewClient(options), nil
}

func (c *Cooldowns) TryStart(ctx context.Context, updateID, chatID int64, _ time.Time) (handler.CooldownReservation, bool, error) {
	token, err := randomToken()
	if err != nil {
		return nil, false, fmt.Errorf("create reservation token: %w", err)
	}
	pendingPrefix := "pending:" + strconv.FormatInt(updateID, 10) + ":"
	reservationToken := pendingPrefix + token
	opCtx, cancel := context.WithTimeout(ctx, redisOperationLimit)
	defer cancel()
	state, err := c.executor.Run(opCtx, reserveScript, []string{c.key(chatID)}, pendingPrefix, reservationToken, reservationTTL.Milliseconds())
	if err != nil {
		return nil, false, fmt.Errorf("reserve cooldown: %w", err)
	}
	switch state {
	case 0:
		return nil, false, nil
	case 1:
		return &reservation{store: c, key: c.key(chatID), token: reservationToken}, true, nil
	case 2:
		return nil, false, errors.New("this update already holds the cooldown reservation")
	default:
		return nil, false, fmt.Errorf("unexpected reservation state %d", state)
	}
}

type reservation struct {
	store *Cooldowns
	key   string
	token string
}

func (r *reservation) Finish(ctx context.Context, _ time.Time, success bool) error {
	commit := 0
	if success {
		commit = 1
	}
	opCtx, cancel := context.WithTimeout(ctx, redisOperationLimit)
	defer cancel()
	changed, err := r.store.executor.Run(opCtx, finishScript, []string{r.key}, r.token, commit, r.store.duration.Milliseconds())
	if err != nil {
		return fmt.Errorf("finalize cooldown: %w", err)
	}
	if success && changed != 1 {
		return errors.New("cooldown reservation expired before commit")
	}
	return nil
}

func (c *Cooldowns) key(chatID int64) string {
	return c.prefix + ":cooldown:" + strconv.FormatInt(chatID, 10)
}

func randomToken() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
