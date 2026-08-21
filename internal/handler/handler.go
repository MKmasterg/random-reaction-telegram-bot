package handler

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/MKmasterg/random-reaction-telegram-bot/internal/domain"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/reaction"
)

const UsageText = "I occasionally add a random Persian or English reaction in group chats.\n\n/reaction — get a random reaction\n/personal — get a reaction using your display name\n/start or /help — show this message\n\nFor automatic replies, disable Group Privacy Mode in BotFather or make me a group administrator."

type Sender interface {
	SendReply(ctx context.Context, chatID int64, replyToMessageID int, text string) error
}

type ReactionGenerator interface {
	General() string
	Personal(name string) string
}

type FloatRandom interface {
	Float64() float64
}

type CooldownStore interface {
	TryStart(ctx context.Context, updateID, chatID int64, now time.Time) (CooldownReservation, bool, error)
}

type CooldownReservation interface {
	Finish(ctx context.Context, now time.Time, success bool) error
}

type Options struct {
	Probability      float64
	ProbabilityScore func(updateID int64) float64
	Random           FloatRandom
	Now              func() time.Time
	Logger           *slog.Logger
}

// Handler applies command, probability, and cooldown policy to domain updates.
type Handler struct {
	sender           Sender
	generator        ReactionGenerator
	cooldowns        CooldownStore
	probability      float64
	probabilityScore func(updateID int64) float64
	random           FloatRandom
	now              func() time.Time
	logger           *slog.Logger
}

func New(sender Sender, generator ReactionGenerator, cooldowns CooldownStore, options Options) *Handler {
	probabilityScore := options.ProbabilityScore
	if probabilityScore == nil {
		probabilityScore = deterministicProbabilityScore
	}
	return &Handler{
		sender:           sender,
		generator:        generator,
		cooldowns:        cooldowns,
		probability:      options.Probability,
		probabilityScore: probabilityScore,
		random:           options.Random,
		now:              options.Now,
		logger:           options.Logger,
	}
}

// Handle processes one update. Ignored updates return nil; delivery failures are
// returned so webhook transport can ask Telegram to retry them.
func (h *Handler) Handle(ctx context.Context, update domain.Update) error {
	if update.Message == nil {
		h.logDecision(update.ID, 0, "ignored_unsupported_update")
		return nil
	}
	message := update.Message
	if message.ChatType == "channel" {
		h.logDecision(update.ID, message.ChatID, "ignored_channel")
		return nil
	}
	if message.From == nil {
		h.logDecision(update.ID, message.ChatID, "ignored_missing_sender")
		return nil
	}
	if message.From.IsBot {
		h.logDecision(update.ID, message.ChatID, "ignored_bot_sender")
		return nil
	}
	if message.IsService || strings.TrimSpace(message.Text) == "" {
		h.logDecision(update.ID, message.ChatID, "ignored_non_text_or_service")
		return nil
	}

	command, isCommand := parseCommand(message.Text)
	if message.ChatType == "private" {
		if isCommand && isKnownCommand(command) {
			return h.sendExplicit(ctx, update.ID, message, UsageText, "private_help")
		} else {
			h.logDecision(update.ID, message.ChatID, "ignored_private_message")
		}
		return nil
	}
	if message.ChatType != "group" && message.ChatType != "supergroup" {
		h.logDecision(update.ID, message.ChatID, "ignored_chat_type")
		return nil
	}

	if isCommand {
		switch command {
		case "reaction":
			return h.sendExplicit(ctx, update.ID, message, h.generator.General(), "command_reaction")
		case "personal":
			name := reaction.DisplayName(message.From)
			return h.sendExplicit(ctx, update.ID, message, h.generator.Personal(name), "command_personal")
		case "start", "help":
			return h.sendExplicit(ctx, update.ID, message, UsageText, "command_help")
		default:
			h.logDecision(update.ID, message.ChatID, "ignored_command")
		}
		return nil
	}

	if h.probabilityScore(update.ID) >= h.probability {
		h.logDecision(update.ID, message.ChatID, "automatic_probability_skip")
		return nil
	}
	startedAt := h.now()
	reservation, started, err := h.cooldowns.TryStart(ctx, update.ID, message.ChatID, startedAt)
	if err != nil {
		h.logger.Error("reserve automatic cooldown", "update_id", update.ID, "chat_id", message.ChatID, "error", err)
		return fmt.Errorf("reserve automatic cooldown: %w", err)
	}
	if !started {
		h.logDecision(update.ID, message.ChatID, "automatic_cooldown_skip")
		return nil
	}

	var text string
	if h.random.Float64() < 0.5 {
		text = h.generator.General()
	} else {
		text = h.generator.Personal(reaction.DisplayName(message.From))
	}
	sendErr := h.sender.SendReply(ctx, message.ChatID, message.ID, text)
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	finishErr := reservation.Finish(finishCtx, h.now(), sendErr == nil)
	cancel()
	if sendErr != nil {
		h.logger.Error("send automatic reaction", "update_id", update.ID, "chat_id", message.ChatID, "error", sendErr)
		if finishErr != nil {
			return errors.Join(fmt.Errorf("send automatic reaction: %w", sendErr), fmt.Errorf("release automatic cooldown: %w", finishErr))
		}
		return fmt.Errorf("send automatic reaction: %w", sendErr)
	}
	if finishErr != nil {
		h.logger.Error("commit automatic cooldown", "update_id", update.ID, "chat_id", message.ChatID, "error", finishErr)
		return fmt.Errorf("commit automatic cooldown: %w", finishErr)
	}
	h.logDecision(update.ID, message.ChatID, "automatic_sent")
	return nil
}

func deterministicProbabilityScore(updateID int64) float64 {
	var input [8]byte
	binary.BigEndian.PutUint64(input[:], uint64(updateID))
	digest := sha256.Sum256(input[:])
	const precision = 53
	value := binary.BigEndian.Uint64(digest[:8]) >> (64 - precision)
	return float64(value) / (1 << precision)
}

func (h *Handler) sendExplicit(ctx context.Context, updateID int64, message *domain.Message, text, decision string) error {
	if err := h.sender.SendReply(ctx, message.ChatID, message.ID, text); err != nil {
		h.logger.Error("send explicit reaction", "update_id", updateID, "chat_id", message.ChatID, "decision", decision, "error", err)
		return fmt.Errorf("send explicit reaction: %w", err)
	}
	h.logDecision(updateID, message.ChatID, decision)
	return nil
}

func (h *Handler) logDecision(updateID, chatID int64, decision string) {
	h.logger.Info("update handled", "update_id", updateID, "chat_id", chatID, "decision", decision)
}

func parseCommand(text string) (string, bool) {
	fields := strings.Fields(text)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return "", false
	}
	command := strings.TrimPrefix(fields[0], "/")
	if at := strings.IndexByte(command, '@'); at >= 0 {
		command = command[:at]
	}
	return strings.ToLower(command), true
}

func isKnownCommand(command string) bool {
	switch command {
	case "reaction", "personal", "start", "help":
		return true
	default:
		return false
	}
}
