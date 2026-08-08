package assistant

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cymonevo/go_template/internal/domain/assistant/builtin"
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
	if op := stringArgValue(args, "operation"); op != "" {
		if prompt := confirmationPromptReminder(op, args); prompt != "" {
			return prompt
		}
	}
	if prompt := confirmationPromptLocationReminder(args); prompt != "" {
		return prompt
	}
	if prompt := confirmationPromptGoogleCalendarMeet(args); prompt != "" {
		return prompt
	}
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

func inferReminderOperation(text string, args map[string]any) string {
	if op := stringArgValue(args, "operation"); op != "" {
		return op
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "list") && strings.Contains(lower, "reminder") {
		return "list"
	}
	if (strings.Contains(lower, "delete") || strings.Contains(lower, "remove")) && strings.Contains(lower, "reminder") {
		return "delete"
	}
	return "create"
}

func inferReminderFilter(text string, args map[string]any) {
	if stringArgValue(args, "filter") != "" {
		return
	}
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "tomorrow"):
		args["filter"] = "tomorrow"
	case strings.Contains(lower, "for today") || (strings.Contains(lower, "today") && !strings.Contains(lower, "all reminders")):
		args["filter"] = "today"
	case strings.Contains(lower, "all"):
		args["filter"] = "all"
	default:
		args["filter"] = "today"
	}
}

func firstMissingReminderArgument(operation string, args map[string]any) (string, string) {
	switch operation {
	case "create":
		if stringArgValue(args, "message") == "" {
			return "message", "What should I remind you about?"
		}
		if stringArgValue(args, "remind_at") == "" {
			return "remind_at", "When should I remind you?"
		}
	case "delete":
		if stringArgValue(args, "message") == "" {
			return "message", "Which reminder should I delete?"
		}
	case "list":
		if stringArgValue(args, "filter") == "" {
			args["filter"] = "today"
		}
	}
	return "", ""
}

func reminderNeedsConfirmation(operation string) bool {
	return operation == "create" || operation == "delete"
}

func confirmationPromptReminder(operation string, args map[string]any) string {
	switch operation {
	case "create":
		message := stringArgValue(args, "message")
		when := builtin.FormatRemindAtForConfirmation(stringArgValue(args, "remind_at"))
		return fmt.Sprintf("Should I set a reminder to %s at %s?", message, when)
	case "delete":
		message := stringArgValue(args, "message")
		return fmt.Sprintf("Should I delete the reminder to %s?", message)
	default:
		return ""
	}
}

func confirmationPromptLocationReminder(args map[string]any) string {
	title := stringArgValue(args, "title")
	mode := stringArgValue(args, "location_mode")
	if title == "" || mode == "" {
		return ""
	}

	radius := 100
	if raw, ok := args["radius_meters"]; ok {
		switch v := raw.(type) {
		case int:
			radius = v
		case float64:
			radius = int(v)
		}
	}

	place := stringArgValue(args, "place_query")
	if mode == builtin.LocationModePlaceKeyword {
		place = builtin.NormalizePlaceKeyword(place)
	}
	if place == "" {
		place = "the specified place"
	}

	return fmt.Sprintf("Should I remind you to %q when you're within %dm of %s?", title, radius, place)
}

func firstMissingLocationReminderArgument(args map[string]any) (string, string) {
	if stringArgValue(args, "title") == "" {
		return "title", "What should I remind you about?"
	}

	mode := stringArgValue(args, "location_mode")
	if mode == "" {
		return "location_mode", "Is this for a specific address or any nearby place?"
	}

	query := stringArgValue(args, "place_query")
	if mode == builtin.LocationModeExact {
		if query == "" || builtin.IsVaguePlaceQuery(query) {
			if builtin.IsVaguePlaceQuery(query) {
				delete(args, "place_query")
			}
			return "place_query", "What is the address?"
		}
		return "", ""
	}

	if mode == builtin.LocationModePlaceKeyword {
		if query == "" {
			return "place_query", "Where can you do that? (e.g. any nearby Alfamart)"
		}
		args["place_query"] = builtin.NormalizePlaceKeyword(query)
		return "", ""
	}

	return "", ""
}

func applyLocationReminderFollowUp(missingArg, text string, args map[string]any) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}

	switch missingArg {
	case "location_mode":
		if mode, query, ok := parseLocationModeReply(text); ok {
			args["location_mode"] = mode
			if query != "" {
				args["place_query"] = query
			}
			return true
		}
		args["location_mode"] = text
		return true
	case "place_query":
		mode := stringArgValue(args, "location_mode")
		if mode == builtin.LocationModePlaceKeyword || looksLikePlaceKeywordReply(text) {
			args["location_mode"] = builtin.LocationModePlaceKeyword
			args["place_query"] = builtin.NormalizePlaceKeyword(text)
			return true
		}
		args["place_query"] = text
		return true
	}
	return false
}

func parseLocationModeReply(text string) (mode, placeQuery string, ok bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	switch lower {
	case "exact", "specific", "specific address", "address":
		return builtin.LocationModeExact, "", true
	case "place_keyword", "nearby", "any nearby", "any nearby place":
		return builtin.LocationModePlaceKeyword, "", true
	}
	if strings.Contains(lower, "nearby") || strings.Contains(lower, "any ") {
		return builtin.LocationModePlaceKeyword, builtin.NormalizePlaceKeyword(text), true
	}
	return "", "", false
}

func looksLikePlaceKeywordReply(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "nearby") || strings.Contains(lower, "any ")
}

func inferLocationReminderFromText(text string, args map[string]any) {
	query := stringArgValue(args, "place_query")
	if query == "" {
		return
	}
	if stringArgValue(args, "location_mode") == "" {
		if looksLikePlaceKeywordReply(query) {
			args["location_mode"] = builtin.LocationModePlaceKeyword
			args["place_query"] = builtin.NormalizePlaceKeyword(query)
		} else if !builtin.IsVaguePlaceQuery(query) {
			args["location_mode"] = builtin.LocationModeExact
		}
	} else if stringArgValue(args, "location_mode") == builtin.LocationModePlaceKeyword {
		args["place_query"] = builtin.NormalizePlaceKeyword(query)
	}
}
