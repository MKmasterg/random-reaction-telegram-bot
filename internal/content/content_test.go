package content

import (
	"strings"
	"testing"
)

func TestLoadEmbedded(t *testing.T) {
	model, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded() error = %v", err)
	}
	if !strings.Contains(model.GeneralReactions[0], "مغزم") {
		t.Error("embedded UTF-8 Persian content was not preserved")
	}
}

func TestLoadAllowsNameOnlyPersonalGroup(t *testing.T) {
	model, err := Load([]byte(`{"general_reactions":["x"],"personal_groups":[{"template":"reaction {name}:"}]}`))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(model.PersonalGroups) != 1 || len(model.PersonalGroups[0].Actions) != 0 {
		t.Fatalf("personal groups = %+v", model.PersonalGroups)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr string
	}{
		{name: "malformed", data: `{`, wantErr: "decode"},
		{name: "empty general", data: `{"general_reactions":[],"personal_groups":[{"template":"{name} {action}","actions":["x"]}]}`, wantErr: "general_reactions"},
		{name: "blank general", data: `{"general_reactions":[" "],"personal_groups":[{"template":"{name} {action}","actions":["x"]}]}`, wantErr: "general_reactions[0]"},
		{name: "empty personal", data: `{"general_reactions":["x"],"personal_groups":[]}`, wantErr: "personal_groups"},
		{name: "missing name", data: `{"general_reactions":["x"],"personal_groups":[{"template":"{action}","actions":["x"]}]}`, wantErr: "{name}"},
		{name: "missing action placeholder", data: `{"general_reactions":["x"],"personal_groups":[{"template":"{name}","actions":["x"]}]}`, wantErr: "{action}"},
		{name: "empty actions", data: `{"general_reactions":["x"],"personal_groups":[{"template":"{name} {action}","actions":[]}]}`, wantErr: "actions"},
		{name: "blank action", data: `{"general_reactions":["x"],"personal_groups":[{"template":"{name} {action}","actions":[""]}]}`, wantErr: "actions[0]"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load([]byte(test.data))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Load() error = %v, want error containing %q", err, test.wantErr)
			}
		})
	}
}

func TestLoadRejectsInvalidUTF8(t *testing.T) {
	data := append([]byte(`{"general_reactions":["`), 0xff)
	data = append(data, []byte(`"],"personal_groups":[]}`)...)
	_, err := Load(data)
	if err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("Load() error = %v, want UTF-8 error", err)
	}
}
