package assistant

import (
	"testing"

	"strings"

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
			{Name: "attendee_name", Required: true, Prompt: "Who would you like to meet with?"},
			{Name: "attendee_email", Required: true, Prompt: "What is {attendee_name}'s email address?"},
		},
	}

	name, prompt := firstMissingArgument(manifest, map[string]any{"attendee_name": "Janet"})
	if name != "attendee_email" || prompt != "What is Janet's email address?" {
		t.Fatalf("unexpected missing arg: %q / %q", name, prompt)
	}
}

func TestConfirmationPromptIncludesAttendeeEmail(t *testing.T) {
	got := confirmationPrompt("Google Meet Scheduler", map[string]any{
		"attendee_name":  "Janet",
		"attendee_email": "janet@gmail.com",
	})
	want := "Should I create a calendar event with Janet at janet@gmail.com?"
	if got != want {
		t.Fatalf("confirmationPrompt() = %q, want %q", got, want)
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

func TestInferReminderOperation(t *testing.T) {
	args := map[string]any{}
	if got := inferReminderOperation("list all reminders for today", args); got != "list" {
		t.Fatalf("got %q, want list", got)
	}
	if got := inferReminderOperation("delete my reminder for milk", args); got != "delete" {
		t.Fatalf("got %q, want delete", got)
	}
	if got := inferReminderOperation("remind me to call mom", args); got != "create" {
		t.Fatalf("got %q, want create", got)
	}
}

func TestFirstMissingReminderArgument(t *testing.T) {
	args := map[string]any{"operation": "create"}
	name, prompt := firstMissingReminderArgument("create", args)
	if name != "message" || prompt != "What should I remind you about?" {
		t.Fatalf("got %q / %q", name, prompt)
	}

	args = map[string]any{"operation": "create", "message": "call mom"}
	name, prompt = firstMissingReminderArgument("create", args)
	if name != "remind_at" || prompt != "When should I remind you?" {
		t.Fatalf("got %q / %q", name, prompt)
	}
}

func TestConfirmationPromptReminder(t *testing.T) {
	got := confirmationPromptReminder("create", map[string]any{
		"message":   "call mom",
		"remind_at": "2026-08-09T14:00:00Z",
	})
	if !strings.Contains(got, "call mom") || !strings.Contains(got, "Should I set a reminder") {
		t.Fatalf("unexpected prompt: %q", got)
	}

	got = confirmationPromptReminder("delete", map[string]any{"message": "call mom"})
	want := "Should I delete the reminder to call mom?"
	if got != want {
		t.Fatalf("confirmationPromptReminder() = %q, want %q", got, want)
	}
}
