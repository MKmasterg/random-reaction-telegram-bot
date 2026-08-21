package content

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Model is the version-controlled reaction content schema.
type Model struct {
	GeneralReactions []string        `json:"general_reactions"`
	PersonalGroups   []PersonalGroup `json:"personal_groups"`
}

// PersonalGroup combines a localized template with optional compatible actions.
type PersonalGroup struct {
	Template string   `json:"template"`
	Actions  []string `json:"actions,omitempty"`
}

//go:embed reactions.json
var embedded []byte

// LoadEmbedded parses and validates the content compiled into the binary.
func LoadEmbedded() (Model, error) {
	return Load(embedded)
}

// Load parses and validates reaction content.
func Load(data []byte) (Model, error) {
	if !utf8.Valid(data) {
		return Model{}, errors.New("reaction content must be valid UTF-8")
	}
	var model Model
	if err := json.Unmarshal(data, &model); err != nil {
		return Model{}, fmt.Errorf("decode reaction content: %w", err)
	}
	if err := Validate(model); err != nil {
		return Model{}, err
	}
	return model, nil
}

// Validate ensures every generation branch can produce a complete reaction.
func Validate(model Model) error {
	if len(model.GeneralReactions) == 0 {
		return errors.New("general_reactions must not be empty")
	}
	for i, reaction := range model.GeneralReactions {
		if strings.TrimSpace(reaction) == "" {
			return fmt.Errorf("general_reactions[%d] must not be empty", i)
		}
	}
	if len(model.PersonalGroups) == 0 {
		return errors.New("personal_groups must not be empty")
	}
	for i, group := range model.PersonalGroups {
		if !strings.Contains(group.Template, "{name}") {
			return fmt.Errorf("personal_groups[%d].template must contain {name}", i)
		}
		hasAction := strings.Contains(group.Template, "{action}")
		if hasAction && len(group.Actions) == 0 {
			return fmt.Errorf("personal_groups[%d].actions must not be empty", i)
		}
		if !hasAction && len(group.Actions) != 0 {
			return fmt.Errorf("personal_groups[%d].template must contain {action} when actions are provided", i)
		}
		for j, action := range group.Actions {
			if strings.TrimSpace(action) == "" {
				return fmt.Errorf("personal_groups[%d].actions[%d] must not be empty", i, j)
			}
		}
	}
	return nil
}
