package domain

// Update contains the small subset of a Telegram update used by the bot.
type Update struct {
	ID      int64
	Message *Message
}

// Message contains the fields needed to decide whether and how to reply.
type Message struct {
	ID        int
	ChatID    int64
	ChatType  string
	Text      string
	From      *User
	IsService bool
}

// User contains the identity fields used to build a display name.
type User struct {
	ID        int64
	IsBot     bool
	FirstName string
	LastName  string
	Username  string
}
