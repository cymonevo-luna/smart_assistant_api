package assistant

import (
	"testing"

	"github.com/cymonevo/go_template/internal/domain/plugin"
)

func TestStripWakeWord(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wakeWord string
		source   MessageSource
		want     string
	}{
		{
			name:     "wake word source strips prefix",
			text:     "Jarvis turn on lights",
			wakeWord: "Jarvis",
			source:   MessageSourceWakeWord,
			want:     "turn on lights",
		},
		{
			name:     "case insensitive strip",
			text:     "jarvis schedule meeting",
			wakeWord: "Jarvis",
			source:   MessageSourceWakeWord,
			want:     "schedule meeting",
		},
		{
			name:     "button without prefix unchanged",
			text:     "turn on lights",
			wakeWord: "Jarvis",
			source:   MessageSourceButton,
			want:     "turn on lights",
		},
		{
			name:     "text starting with wake word stripped on button source",
			text:     "Jarvis what's the weather",
			wakeWord: "Jarvis",
			source:   MessageSourceButton,
			want:     "what's the weather",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripWakeWord(tt.text, tt.wakeWord, tt.source)
			if got != tt.want {
				t.Fatalf("stripWakeWord() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFirstMissingArgument(t *testing.T) {
	manifest := plugin.PluginManifest{
		Arguments: []plugin.ManifestArgument{
			{Name: "title", Required: true, Prompt: "What is the title?"},
			{Name: "attendee_email", Required: true, Prompt: "What is Janet's email address?"},
		},
	}

	name, prompt := firstMissingArgument(manifest, map[string]any{"title": "Meeting"})
	if name != "attendee_email" || prompt != "What is Janet's email address?" {
		t.Fatalf("unexpected missing arg: %q / %q", name, prompt)
	}
}

func TestConfirmationParsing(t *testing.T) {
	if !isConfirmationYes("yes") || !isConfirmationYes("OK") {
		t.Fatal("expected yes responses to be recognized")
	}
	if !isConfirmationNo("no") || !isConfirmationNo("cancel") {
		t.Fatal("expected no responses to be recognized")
	}
}
