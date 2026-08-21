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
	model, err := content.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	random := &intSequence{values: []int{0, 8, 0, 0, 1, 11}}
	generator := New(model, random)

	if got := generator.General(); !strings.Contains(got, "مغزم") {
		t.Errorf("Persian General() = %q", got)
	}
	if got := generator.General(); !strings.Contains(got, "brain cell") {
		t.Errorf("English General() = %q", got)
	}
	if got := generator.Personal("نیما"); strings.Contains(got, "{name}") || strings.Contains(got, "{action}") || !strings.Contains(got, "نیما") {
		t.Errorf("Persian Personal() = %q", got)
	}
	if got := generator.Personal("Sam"); strings.Contains(got, "{name}") || strings.Contains(got, "{action}") || !strings.Contains(got, "Sam when") {
		t.Errorf("English Personal() = %q", got)
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
