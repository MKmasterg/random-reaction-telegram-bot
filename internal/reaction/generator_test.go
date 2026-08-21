package reaction

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/MKmasterg/random-reaction-telegram-bot/internal/content"
	"github.com/MKmasterg/random-reaction-telegram-bot/internal/domain"
)

type intSequence struct {
	values []int
	index  int
}

func (s *intSequence) IntN(n int) int {
	value := s.values[s.index]
	s.index++
	if value < 0 || value >= n {
		panic("test random value outside bound")
	}
	return value
}

func TestGeneratorSelectsEveryContentBranch(t *testing.T) {
	model := content.Model{
		GeneralReactions: []string{"general zero", "general one"},
		PersonalGroups: []content.PersonalGroup{
			{Template: "{name} when {action}", Actions: []string{"first", "second"}},
			{Template: "reaction {name}:"},
		},
	}
	random := &intSequence{values: []int{0, 1, 0, 1, 1}}
	generator := New(model, random)

	if got := generator.General(); got != "general zero" {
		t.Errorf("first General() = %q", got)
	}
	if got := generator.General(); got != "general one" {
		t.Errorf("second General() = %q", got)
	}
	if got := generator.Personal("Sam"); got != "Sam when second" {
		t.Errorf("action Personal() = %q", got)
	}
	if got := generator.Personal("مریم"); got != "reaction مریم:" {
		t.Errorf("name-only Personal() = %q", got)
	}
}

func TestPersonalWithoutAction(t *testing.T) {
	model := content.Model{PersonalGroups: []content.PersonalGroup{{Template: "reaction {name}:"}}}
	generator := New(model, &intSequence{values: []int{0}})
	if got := generator.Personal("A\nB"); got != "reaction A B:" {
		t.Errorf("Personal() = %q", got)
	}
}

func TestSanitizeName(t *testing.T) {
	if got := SanitizeName("  Alice\n\tBob\u202e  "); got != "Alice Bob" {
		t.Errorf("SanitizeName() = %q, want %q", got, "Alice Bob")
	}
	if got := SanitizeName("\x00\n"); got != fallbackDisplayName {
		t.Errorf("empty SanitizeName() = %q", got)
	}
	long := strings.Repeat("ژ", 80)
	if got := SanitizeName(long); utf8.RuneCountInString(got) != maxDisplayNameRunes {
		t.Errorf("SanitizeName() rune count = %d, want %d", utf8.RuneCountInString(got), maxDisplayNameRunes)
	}
}

func TestDisplayNameFallbackOrder(t *testing.T) {
	tests := []struct {
		name string
		user *domain.User
		want string
	}{
		{name: "full name", user: &domain.User{FirstName: "Ada", LastName: "Lovelace", Username: "ada"}, want: "Ada Lovelace"},
		{name: "username", user: &domain.User{Username: "ada"}, want: "@ada"},
		{name: "neutral", user: &domain.User{}, want: fallbackDisplayName},
		{name: "nil", user: nil, want: fallbackDisplayName},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := DisplayName(test.user); got != test.want {
				t.Errorf("DisplayName() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPersonalSanitizesName(t *testing.T) {
	model := content.Model{PersonalGroups: []content.PersonalGroup{{Template: "{name}: {action}", Actions: []string{"acts"}}}}
	generator := New(model, &intSequence{values: []int{0, 0}})
	if got := generator.Personal("A\nB"); got != "A B: acts" {
		t.Errorf("Personal() = %q", got)
	}
}
