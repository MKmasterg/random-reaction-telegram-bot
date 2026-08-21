package telegram

import (
	"context"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/MKmasterg/random-reaction-telegram-bot/internal/domain"
)

type messageAPI interface {
	SendMessage(ctx context.Context, params *tgbot.SendMessageParams) (*models.Message, error)
}

// Delivery adapts the Telegram client to the domain reply interface.
type Delivery struct {
	api messageAPI
}

func NewDelivery(api messageAPI) *Delivery {
	return &Delivery{api: api}
}

func (d *Delivery) SendReply(ctx context.Context, chatID int64, replyToMessageID int, text string) error {
	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := d.api.SendMessage(sendCtx, &tgbot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
		ReplyParameters: &models.ReplyParameters{
			MessageID: replyToMessageID,
		},
	})
	return err
}

// FromUpdate removes Telegram SDK details from the domain handler.
func FromUpdate(update *models.Update) domain.Update {
	if update == nil {
		return domain.Update{}
	}
	converted := domain.Update{ID: update.ID}
	if update.Message == nil {
		return converted
	}
	message := update.Message
	converted.Message = &domain.Message{
		ID:        message.ID,
		ChatID:    message.Chat.ID,
		ChatType:  string(message.Chat.Type),
		Text:      message.Text,
		IsService: message.Text == "",
	}
	if message.From != nil {
		converted.Message.From = &domain.User{
			ID:        message.From.ID,
			IsBot:     message.From.IsBot,
			FirstName: message.From.FirstName,
			LastName:  message.From.LastName,
			Username:  message.From.Username,
		}
	}
	return converted
}
