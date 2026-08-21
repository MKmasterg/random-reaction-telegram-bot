package handler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/MKmasterg/random-reaction-telegram-bot/internal/domain"
)

type sentReply struct {
	chatID    int64
	messageID int
	text      string
}

type fakeSender struct {
	mu       sync.Mutex
	replies  []sentReply
	failNext int
}

func (s *fakeSender) SendReply(_ context.Context, chatID int64, messageID int, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failNext > 0 {
		s.failNext--
		return errors.New("send failed")
	}
	s.replies = append(s.replies, sentReply{chatID: chatID, messageID: messageID, text: text})
	return nil
}

type fakeGenerator struct {
	general       string
	personal      string
	personalNames []string
}

func (g *fakeGenerator) General() string { return g.general }
func (g *fakeGenerator) Personal(name string) string {
	g.personalNames = append(g.personalNames, name)
	return g.personal
}

type floatSequence struct {
	values []float64
	index  int
}

func (s *floatSequence) Float64() float64 {
	if s.index >= len(s.values) {
		panic("test random sequence exhausted")
	}
	value := s.values[s.index]
	s.index++
	return value
}

type fakeClock struct{ current time.Time }

func (c *fakeClock) Now() time.Time { return c.current }

func TestCommandsReplyToTriggeringMessage(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantText string
		wantName string
	}{
		{name: "reaction", text: "/reaction", wantText: "general"},
		{name: "personal", text: "/personal extra", wantText: "personal", wantName: "Ada Lovelace"},
		{name: "start", text: "/start", wantText: UsageText},
		{name: "help with mention", text: "/help@my_bot", wantText: UsageText},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sender := &fakeSender{}
			generator := &fakeGenerator{general: "general", personal: "personal"}
			h := newTestHandler(sender, generator, &floatSequence{}, &fakeClock{current: time.Unix(100, 0)}, 0.5)
			update := groupUpdate(41, 9001, test.text)
			update.Message.From.FirstName = "Ada"
			update.Message.From.LastName = "Lovelace"

			h.Handle(context.Background(), update)

			if len(sender.replies) != 1 {
				t.Fatalf("reply count = %d, want 1", len(sender.replies))
			}
			reply := sender.replies[0]
			if reply.chatID != 41 || reply.messageID != 9001 || reply.text != test.wantText {
				t.Errorf("reply = %+v, want chat=41 message=9001 text=%q", reply, test.wantText)
			}
			if test.wantName != "" && (len(generator.personalNames) != 1 || generator.personalNames[0] != test.wantName) {
				t.Errorf("personal names = %v, want [%q]", generator.personalNames, test.wantName)
			}
		})
	}
}

func TestPrivateChatsReceiveOnlyExplanation(t *testing.T) {
	for _, command := range []string{"/start", "/help", "/reaction", "/personal"} {
		t.Run(command, func(t *testing.T) {
			sender := &fakeSender{}
			generator := &fakeGenerator{general: "general", personal: "personal"}
			h := newTestHandler(sender, generator, &floatSequence{}, &fakeClock{current: time.Unix(100, 0)}, 1)
			update := groupUpdate(1, 2, command)
			update.Message.ChatType = "private"

			h.Handle(context.Background(), update)

			if len(sender.replies) != 1 || sender.replies[0].text != UsageText {
				t.Fatalf("replies = %+v, want one usage reply", sender.replies)
			}
			if len(generator.personalNames) != 0 {
				t.Errorf("private command invoked personal generator: %v", generator.personalNames)
			}
		})
	}
}

func TestUnsupportedUpdatesAreIgnored(t *testing.T) {
	tests := []struct {
		name   string
		update domain.Update
	}{
		{name: "missing message", update: domain.Update{ID: 1}},
		{name: "channel", update: mutate(groupUpdate(1, 1, "hello"), func(message *domain.Message) { message.ChatType = "channel" })},
		{name: "missing sender", update: mutate(groupUpdate(1, 1, "hello"), func(message *domain.Message) { message.From = nil })},
		{name: "bot sender", update: mutate(groupUpdate(1, 1, "hello"), func(message *domain.Message) { message.From.IsBot = true })},
		{name: "service", update: mutate(groupUpdate(1, 1, "joined"), func(message *domain.Message) { message.IsService = true })},
		{name: "empty text", update: mutate(groupUpdate(1, 1, " "), func(message *domain.Message) {})},
		{name: "unsupported chat", update: mutate(groupUpdate(1, 1, "hello"), func(message *domain.Message) { message.ChatType = "sender" })},
		{name: "unknown command", update: groupUpdate(1, 1, "/unknown")},
		{name: "private ordinary", update: mutate(groupUpdate(1, 1, "hello"), func(message *domain.Message) { message.ChatType = "private" })},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sender := &fakeSender{}
			h := newTestHandler(sender, &fakeGenerator{}, &floatSequence{}, &fakeClock{current: time.Unix(100, 0)}, 1)
			h.Handle(context.Background(), test.update)
			if len(sender.replies) != 0 {
				t.Errorf("replies = %+v, want none", sender.replies)
			}
		})
	}
}

func TestAutomaticProbabilityBoundary(t *testing.T) {
	t.Run("equal skips", func(t *testing.T) {
		sender := &fakeSender{}
		h := newTestHandler(sender, &fakeGenerator{}, &floatSequence{values: []float64{0.5}}, &fakeClock{current: time.Unix(100, 0)}, 0.5)
		h.Handle(context.Background(), groupUpdate(1, 1, "hello"))
		if len(sender.replies) != 0 {
			t.Fatalf("reply count = %d, want 0", len(sender.replies))
		}
	})

	t.Run("one always sends", func(t *testing.T) {
		sender := &fakeSender{}
		h := newTestHandler(sender, &fakeGenerator{general: "general"}, &floatSequence{values: []float64{0.999999, 0.1}}, &fakeClock{current: time.Unix(100, 0)}, 1)
		h.Handle(context.Background(), groupUpdate(1, 7, "hello"))
		if len(sender.replies) != 1 || sender.replies[0].messageID != 7 {
			t.Fatalf("replies = %+v, want reply to 7", sender.replies)
		}
	})
}

func TestAutomaticCooldownIsolationAndBranches(t *testing.T) {
	sender := &fakeSender{}
	generator := &fakeGenerator{general: "general", personal: "personal"}
	random := &floatSequence{values: []float64{
		0, 0.1, // group 10 sends general
		0,      // group 10 passes probability but is cooling down
		0, 0.1, // group 20 is isolated and sends general
		0, 0.9, // group 10 sends personal after cooldown
	}}
	clock := &fakeClock{current: time.Unix(100, 0)}
	h := newTestHandler(sender, generator, random, clock, 1)

	h.Handle(context.Background(), groupUpdate(10, 1, "first"))
	h.Handle(context.Background(), groupUpdate(10, 2, "second"))
	h.Handle(context.Background(), groupUpdate(20, 3, "third"))
	clock.current = clock.current.Add(5 * time.Minute)
	h.Handle(context.Background(), groupUpdate(10, 4, "fourth"))

	if len(sender.replies) != 3 {
		t.Fatalf("replies = %+v, want 3", sender.replies)
	}
	if sender.replies[0].text != "general" || sender.replies[1].chatID != 20 || sender.replies[2].text != "personal" {
		t.Errorf("unexpected replies: %+v", sender.replies)
	}
	if len(generator.personalNames) != 1 || generator.personalNames[0] != "Test User" {
		t.Errorf("personal names = %v", generator.personalNames)
	}
}

func TestExplicitSendFailureIsReturned(t *testing.T) {
	sender := &fakeSender{failNext: 1}
	h := newTestHandler(sender, &fakeGenerator{general: "general"}, &floatSequence{}, &fakeClock{current: time.Unix(100, 0)}, 1)

	if err := h.Handle(context.Background(), groupUpdate(10, 1, "/reaction")); err == nil {
		t.Fatal("failed explicit send returned nil error")
	}
}

func TestCooldownStartsOnlyAfterSuccessfulSend(t *testing.T) {
	sender := &fakeSender{failNext: 1}
	random := &floatSequence{values: []float64{0, 0.1, 0, 0.1}}
	h := newTestHandler(sender, &fakeGenerator{general: "general"}, random, &fakeClock{current: time.Unix(100, 0)}, 1)

	if err := h.Handle(context.Background(), groupUpdate(10, 1, "first")); err == nil {
		t.Fatal("failed automatic send returned nil error")
	}
	if err := h.Handle(context.Background(), groupUpdate(10, 2, "second")); err != nil {
		t.Fatalf("successful automatic retry returned error: %v", err)
	}

	if len(sender.replies) != 1 || sender.replies[0].messageID != 2 {
		t.Fatalf("replies = %+v, want successful retry on message 2", sender.replies)
	}
}

func TestExplicitCommandBypassesCooldown(t *testing.T) {
	sender := &fakeSender{}
	h := newTestHandler(sender, &fakeGenerator{general: "general"}, &floatSequence{values: []float64{0, 0.1}}, &fakeClock{current: time.Unix(100, 0)}, 1)

	h.Handle(context.Background(), groupUpdate(10, 1, "ordinary"))
	h.Handle(context.Background(), groupUpdate(10, 2, "/reaction"))

	if len(sender.replies) != 2 || sender.replies[1].messageID != 2 {
		t.Fatalf("replies = %+v, want command during cooldown", sender.replies)
	}
}

func newTestHandler(sender Sender, generator ReactionGenerator, random FloatRandom, clock *fakeClock, probability float64) *Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(sender, generator, NewCooldowns(5*time.Minute), Options{
		Probability: probability,
		Random:      random,
		Now:         clock.Now,
		Logger:      logger,
	})
}

func groupUpdate(chatID int64, messageID int, text string) domain.Update {
	return domain.Update{
		ID: int64(messageID + 100),
		Message: &domain.Message{
			ID:       messageID,
			ChatID:   chatID,
			ChatType: "supergroup",
			Text:     text,
			From: &domain.User{
				ID:        99,
				FirstName: "Test",
				LastName:  "User",
			},
		},
	}
}

func mutate(update domain.Update, change func(*domain.Message)) domain.Update {
	change(update.Message)
	return update
}
