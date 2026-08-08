package assistant

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cymonevo/go_template/internal/domain/plugin"
)

var confirmationYes = regexp.MustCompile(`(?i)^\s*(yes|yeah|yep|y|confirm|ok|okay|sure|do it|go ahead)\s*[.!]?\s*$`)
var confirmationNo = regexp.MustCompile(`(?i)^\s*(no|nah|nope|n|cancel|stop|don't|do not)\s*[.!]?\s*$`)

// stripWakeWord removes a leading wake word from text when appropriate.
func stripWakeWord(text string, wakeWord string, source MessageSource) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || wakeWord == "" {
		return trimmed
	}

	lowerText := strings.ToLower(trimmed)
	lowerWake := strings.ToLower(strings.TrimSpace(wakeWord))

	shouldStrip := source == MessageSourceWakeWord || strings.HasPrefix(lowerText, lowerWake)
	if !shouldStrip {
		return trimmed
	}

	if strings.HasPrefix(lowerText, lowerWake) {
		if len(trimmed) > len(lowerWake) && !isWordBoundary(lowerWake[len(lowerWake)-1], trimmed[len(lowerWake)]) {
			return trimmed
		}
		return strings.TrimSpace(trimmed[len(lowerWake):])
	}
	return trimmed
}

func isWordBoundary(before, after byte) bool {
	return !isAlphaNum(before) || !isAlphaNum(after)
}

func isAlphaNum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func isConfirmationYes(text string) bool {
	return confirmationYes.MatchString(strings.TrimSpace(text))
}

func isConfirmationNo(text string) bool {
	return confirmationNo.MatchString(strings.TrimSpace(text))
}

func firstMissingArgument(manifest plugin.PluginManifest, args map[string]any) (string, string) {
	for _, arg := range manifest.Arguments {
		if !arg.Required {
			continue
		}
		val, ok := args[arg.Name]
		if !ok || isEmptyValue(val) {
			return arg.Name, interpolatePrompt(arg.Prompt, args)
		}
	}
	return "", ""
}

func isEmptyValue(v any) bool {
	if v == nil {
		return true
	}
	switch s := v.(type) {
	case string:
		return strings.TrimSpace(s) == ""
	default:
		return false
	}
}

func confirmationPrompt(pluginName string, args map[string]any) string {
	email := stringArgValue(args, "attendee_email")
	if email != "" {
		name := stringArgValue(args, "attendee_name")
		if name != "" {
			return fmt.Sprintf("Should I create a calendar event with %s at %s?", name, email)
		}
		return fmt.Sprintf("Should I create a calendar event with %s?", email)
	}
	return "Should I go ahead with " + pluginName + "?"
}

var promptPlaceholder = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

func interpolatePrompt(prompt string, args map[string]any) string {
	if prompt == "" || args == nil {
		return prompt
	}
	return promptPlaceholder.ReplaceAllStringFunc(prompt, func(match string) string {
		key := strings.Trim(match, "{}")
		if val := stringArgValue(args, key); val != "" {
			return val
		}
		return match
	})
}

func stringArgValue(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	val, ok := args[key]
	if !ok || val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
