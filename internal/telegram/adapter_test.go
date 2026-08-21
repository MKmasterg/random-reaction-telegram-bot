package telegram

import (
	"context"
	"testing"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type fakeMessageAPI struct {
	params *tgbot.SendMessageParams
	err    error
}

func (a *fakeMessageAPI) SendMessage(_ context.Context, params *tgbot.SendMessageParams) (*models.Message, error) {
	a.params = params
	return &models.Message{}, a.err
}

func TestDeliverySendsPlainReply(t *testing.T) {
	api := &fakeMessageAPI{}
	delivery := NewDelivery(api)
	if err := delivery.SendReply(context.Background(), -1001, 77, "hello <name>"); err != nil {
		t.Fatalf("SendReply() error = %v", err)
	}
	if api.params == nil {
		t.Fatal("SendMessage() was not called")
	}
	if api.params.ChatID != int64(-1001) || api.params.Text != "hello <name>" {
		t.Errorf("params = %+v", api.params)
	}
	if api.params.ReplyParameters == nil || api.params.ReplyParameters.MessageID != 77 {
		t.Errorf("ReplyParameters = %+v, want message 77", api.params.ReplyParameters)
	}
	if api.params.ParseMode != "" {
		t.Errorf("ParseMode = %q, want plain text", api.params.ParseMode)
	}
}

func TestFromUpdate(t *testing.T) {
	update := &models.Update{
		ID: 12,
		Message: &models.Message{
			ID:   34,
			Text: "hello",
			Chat: models.Chat{ID: -99, Type: models.ChatTypeSupergroup},
			From: &models.User{ID: 7, IsBot: false, FirstName: "Ada", LastName: "L", Username: "ada"},
		},
	}
	converted := FromUpdate(update)
	if converted.ID != 12 || converted.Message == nil {
		t.Fatalf("FromUpdate() = %+v", converted)
	}
	if converted.Message.ID != 34 || converted.Message.ChatID != -99 || converted.Message.ChatType != "supergroup" || converted.Message.Text != "hello" {
		t.Errorf("message = %+v", converted.Message)
	}
	if converted.Message.From == nil || converted.Message.From.FirstName != "Ada" || converted.Message.From.Username != "ada" {
		t.Errorf("sender = %+v", converted.Message.From)
	}
}

func TestFromUpdateIgnoresUnsupported(t *testing.T) {
	if converted := FromUpdate(nil); converted.Message != nil {
		t.Errorf("nil update converted to %+v", converted)
	}
	channelPost := &models.Update{ID: 1, ChannelPost: &models.Message{ID: 2}}
	if converted := FromUpdate(channelPost); converted.ID != 1 || converted.Message != nil {
		t.Errorf("channel post converted to %+v", converted)
	}
}
