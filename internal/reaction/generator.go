package reaction

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/MKmasterg/random-reaction-telegram-bot/internal/content"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/domain"
)

const (
	maxDisplayNameRunes = 64
	fallbackDisplayName = "someone"
)

// IntRandom supplies bounded random integers.
type IntRandom interface {
	IntN(n int) int
}

// Generator selects reactions from validated content.
type Generator struct {
	content content.Model
	random  IntRandom
}

func New(model content.Model, random IntRandom) *Generator {
	return &Generator{content: model, random: random}
}

func (g *Generator) General() string {
	return g.content.GeneralReactions[g.random.IntN(len(g.content.GeneralReactions))]
}

func (g *Generator) Personal(name string) string {
	group := g.content.PersonalGroups[g.random.IntN(len(g.content.PersonalGroups))]
	result := strings.ReplaceAll(group.Template, "{name}", SanitizeName(name))
	if strings.Contains(result, "{action}") {
		action := group.Actions[g.random.IntN(len(group.Actions))]
		result = strings.ReplaceAll(result, "{action}", action)
	}
	return result
}

// DisplayName chooses Telegram identity fields in a predictable order.
func DisplayName(user *domain.User) string {
	if user == nil {
		return fallbackDisplayName
	}
	if fullName := normalizeName(user.FirstName + " " + user.LastName); fullName != "" {
		return fullName
	}
	if username := normalizeName(user.Username); username != "" {
		return SanitizeName("@" + username)
	}
	return fallbackDisplayName
}

// SanitizeName removes control characters, normalizes whitespace, and limits
// user-provided names before they are inserted into plain Telegram text.
func SanitizeName(name string) string {
	name = normalizeName(name)
	if name == "" {
		return fallbackDisplayName
	}
	return name
}

func normalizeName(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || !utf8.ValidRune(r) {
			return ' '
		}
		return r
	}, name)
	name = strings.Join(strings.Fields(name), " ")
	runes := []rune(name)
	if len(runes) > maxDisplayNameRunes {
		name = string(runes[:maxDisplayNameRunes])
	}
	return name
}
