package assistant

import (
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
			return arg.Name, arg.Prompt
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
	return "Should I go ahead with " + pluginName + "?"
}
