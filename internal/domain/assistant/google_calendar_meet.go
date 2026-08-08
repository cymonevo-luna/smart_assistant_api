package assistant

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cymonevo/go_template/internal/domain/assistant/builtin"
	"github.com/cymonevo/go_template/internal/domain/calendar/availability"
	"github.com/cymonevo/go_template/internal/domain/plugin"
)

const (
	internalAttendeeEmailKey = "_attendee_email"
	meetingTimezone          = "Asia/Jakarta"
)

var recommendationIntentPattern = regexp.MustCompile(`(?i)(find\s+a\s+time|pick\s+a\s+time|find/pick\s+a\s+time|suggest\s+a\s+time|any\s+time|you\s+pick|pick\s+one)`)

func isGoogleCalendarMeetPlugin(p *plugin.Plugin) bool {
	if p.Slug == builtin.GoogleCalendarMeetSlug {
		return true
	}
	adapter, _ := p.Manifest.Executor.Config["builtin_adapter"].(string)
	return adapter == builtin.AdapterGoogleCalendarMeet
}

func normalizeGoogleCalendarMeetArgs(args map[string]any) {
	names := normalizeAttendeeNames(args)
	args["attendee_names"] = names

	emails := normalizeAttendeeEmails(args)
	if emails == nil {
		emails = []string{}
	}
	args["attendee_emails"] = emails
}

func normalizeAttendeeNames(args map[string]any) []string {
	if names := stringSliceFromArg(args, "attendee_names"); len(names) > 0 {
		return names
	}
	if legacy := stringArgValue(args, "attendee_name"); legacy != "" {
		return splitAttendeeNames(legacy)
	}
	return nil
}

func normalizeAttendeeEmails(args map[string]any) []string {
	if emails := stringSliceFromArg(args, "attendee_emails"); len(emails) > 0 {
		return emails
	}
	if legacy := stringArgValue(args, "attendee_email"); legacy != "" {
		return []string{legacy}
	}
	return nil
}

func splitAttendeeNames(raw string) []string {
	raw = strings.ReplaceAll(raw, " and ", ",")
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func stringSliceFromArg(args map[string]any, key string) []string {
	if args == nil {
		return nil
	}
	val, ok := args[key]
	if !ok || val == nil {
		return nil
	}
	switch v := val.(type) {
	case string:
		return splitAttendeeNames(v)
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return splitAttendeeNames(fmt.Sprint(v))
	}
}

func nextMissingAttendeeEmail(args map[string]any) (name string, ok bool) {
	names := stringSliceFromArg(args, "attendee_names")
	emails := stringSliceFromArg(args, "attendee_emails")
	if len(emails) >= len(names) {
		return "", false
	}
	return names[len(emails)], true
}

func appendAttendeeEmail(args map[string]any, email string) {
	emails := stringSliceFromArg(args, "attendee_emails")
	args["attendee_emails"] = append(emails, strings.TrimSpace(email))
}

func emailCollectPrompt(name string) string {
	return fmt.Sprintf("What is %s's email address?", name)
}

func isRecommendationIntent(text string) bool {
	return recommendationIntentPattern.MatchString(strings.TrimSpace(text))
}

var explicitTimePattern = regexp.MustCompile(`(?i)(\b\d{1,2}\s*(:\d{2})?\s*(am|pm)\b|\b(at)\b|\b(monday|tuesday|wednesday|thursday|friday|saturday|sunday|tomorrow|today)\b)`)

func isExplicitDateTime(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || isRecommendationIntent(text) {
		return false
	}
	return explicitTimePattern.MatchString(text)
}

func scheduleTimePrompt(manifest plugin.PluginManifest) string {
	for _, arg := range manifest.Arguments {
		if arg.Name == "start_time" && strings.TrimSpace(arg.Prompt) != "" {
			return arg.Prompt
		}
	}
	return "When should I set the schedule?"
}

func confirmationPromptGoogleCalendarMeet(args map[string]any) string {
	startTime := stringArgValue(args, "start_time")
	if startTime == "" {
		return ""
	}
	loc, err := time.LoadLocation(meetingTimezone)
	if err != nil {
		loc = time.UTC
	}
	t, err := builtin.ParseDateTime(startTime, loc)
	if err != nil {
		return ""
	}
	t = t.In(loc)
	return fmt.Sprintf("Is %s at %s okay?", t.Format("Monday"), t.Format("3 PM"))
}

func parseExplicitMeetingTime(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	loc, err := time.LoadLocation(meetingTimezone)
	if err != nil {
		loc = time.UTC
	}
	if t, err := builtin.ParseDateTime(text, loc); err == nil {
		return t.In(loc).Format(time.RFC3339)
	}
	return parseNaturalMeetingTime(text, loc)
}

func parseNaturalMeetingTime(text string, loc *time.Location) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	hour := 14
	minute := 0

	if idx := strings.Index(lower, " at "); idx >= 0 {
		segment := lower[idx+4:]
		for _, suffix := range []string{" today", " tomorrow"} {
			if cut := strings.Index(segment, suffix); cut >= 0 {
				segment = segment[:cut]
			}
		}
		if parsedHour, parsedMinute, ok := parseMeetingClock(strings.TrimSpace(segment)); ok {
			hour, minute = parsedHour, parsedMinute
		}
	}

	now := time.Now().In(loc)
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)

	weekdays := map[string]time.Weekday{
		"monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday,
		"thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday, "sunday": time.Sunday,
	}
	for name, weekday := range weekdays {
		if strings.Contains(lower, name) {
			for d := 0; d < 7; d++ {
				candidate := now.AddDate(0, 0, d)
				if candidate.Weekday() == weekday {
					target = time.Date(candidate.Year(), candidate.Month(), candidate.Day(), hour, minute, 0, 0, loc)
					break
				}
			}
			break
		}
	}

	if strings.Contains(lower, "tomorrow") {
		target = target.Add(24 * time.Hour)
	} else if !strings.Contains(lower, "today") && target.Before(now) {
		target = target.Add(24 * time.Hour)
	}

	return target.Format(time.RFC3339)
}

func parseMeetingClock(segment string) (hour, minute int, ok bool) {
	segment = strings.TrimSpace(strings.ToLower(segment))
	segment = strings.ReplaceAll(segment, ".", "")
	isPM := strings.Contains(segment, "pm")
	isAM := strings.Contains(segment, "am")
	segment = strings.ReplaceAll(segment, "pm", "")
	segment = strings.ReplaceAll(segment, "am", "")
	segment = strings.TrimSpace(segment)
	segment = strings.ReplaceAll(segment, " ", "")

	if strings.Contains(segment, ":") {
		parts := strings.SplitN(segment, ":", 2)
		if len(parts) != 2 {
			return 0, 0, false
		}
		if _, err := fmt.Sscanf(parts[0], "%d", &hour); err != nil {
			return 0, 0, false
		}
		if _, err := fmt.Sscanf(parts[1], "%d", &minute); err != nil {
			return 0, 0, false
		}
	} else if _, err := fmt.Sscanf(segment, "%d", &hour); err != nil {
		return 0, 0, false
	}

	if isPM && hour < 12 {
		hour += 12
	}
	if isAM && hour == 12 {
		hour = 0
	}
	return hour, minute, true
}

func stripInternalArgs(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for k, v := range args {
		if strings.HasPrefix(k, "_") {
			continue
		}
		out[k] = v
	}
	return out
}

const noAvailableSlotText = "I couldn't find a mutual time in the next two weeks during working hours. Please try a different time or attendees."

func (s *Service) recommendMeetingSlot(
	ctx context.Context,
	userID string,
	eligible eligiblePlugin,
	args map[string]any,
) (Reply, *PendingAction, SessionStatus, error) {
	if s.availability == nil {
		return Reply{
			Type: ReplyTypeText,
			Text: "I can't look up availability right now. Please try again in a moment.",
		}, nil, SessionStatusActive, nil
	}

	emails := stringSliceFromArg(args, "attendee_emails")
	slot, err := s.availability.RecommendSlot(ctx, userID, availability.RecommendSlotInput{
		AttendeeEmails: emails,
	})
	if errors.Is(err, availability.ErrNoAvailableSlot) {
		return Reply{
			Type: ReplyTypeText,
			Text: noAvailableSlotText,
		}, nil, SessionStatusActive, nil
	}
	if err != nil {
		return Reply{
			Type: ReplyTypeText,
			Text: "I had trouble checking calendars. Please try again in a moment.",
		}, nil, SessionStatusActive, nil
	}

	loc, err := time.LoadLocation(meetingTimezone)
	if err != nil {
		loc = time.UTC
	}
	args["start_time"] = slot.In(loc).Format(time.RFC3339)
	delete(args, "_recommendation_requested")

	return s.googleCalendarMeetConfirmation(eligible, args)
}

func (s *Service) googleCalendarMeetConfirmation(
	eligible eligiblePlugin,
	args map[string]any,
) (Reply, *PendingAction, SessionStatus, error) {
	pending := &PendingAction{
		PluginSlug:           eligible.catalog.Slug,
		PluginID:             eligible.catalog.ID,
		InstallID:            eligible.install.ID,
		Arguments:            args,
		AwaitingConfirmation: true,
	}
	return Reply{
		Type: ReplyTypeConfirmation,
		Text: confirmationPromptGoogleCalendarMeet(args),
		Action: &ActionInfo{
			PluginSlug: eligible.catalog.Slug,
			Status:     ActionStatusPending,
		},
	}, pending, SessionStatusActive, nil
}

func (s *Service) advanceGoogleCalendarMeet(
	ctx context.Context,
	userID string,
	eligible eligiblePlugin,
	args map[string]any,
	text string,
) (Reply, *PendingAction, SessionStatus, error) {
	normalizeGoogleCalendarMeetArgs(args)

	if isRecommendationIntent(text) {
		args["_recommendation_requested"] = true
	}

	if name, ok := nextMissingAttendeeEmail(args); ok {
		pending := &PendingAction{
			PluginSlug:      eligible.catalog.Slug,
			PluginID:        eligible.catalog.ID,
			InstallID:       eligible.install.ID,
			Arguments:       args,
			MissingArgument: internalAttendeeEmailKey,
		}
		return Reply{
			Type: ReplyTypeFollowUp,
			Text: emailCollectPrompt(name),
			Action: &ActionInfo{
				PluginSlug: eligible.catalog.Slug,
				Status:     ActionStatusPending,
			},
		}, pending, SessionStatusActive, nil
	}

	if stringArgValue(args, "start_time") == "" {
		if isRecommendationIntent(text) || args["_recommendation_requested"] == true {
			return s.recommendMeetingSlot(ctx, userID, eligible, args)
		}
		if isExplicitDateTime(text) {
			args["start_time"] = parseExplicitMeetingTime(text)
		} else {
			pending := &PendingAction{
				PluginSlug:      eligible.catalog.Slug,
				PluginID:        eligible.catalog.ID,
				InstallID:       eligible.install.ID,
				Arguments:       args,
				MissingArgument: "start_time",
			}
			return Reply{
				Type: ReplyTypeFollowUp,
				Text: scheduleTimePrompt(eligible.catalog.Manifest),
				Action: &ActionInfo{
					PluginSlug: eligible.catalog.Slug,
					Status:     ActionStatusPending,
				},
			}, pending, SessionStatusActive, nil
		}
	}

	if eligible.catalog.Manifest.ConfirmationRequired {
		return s.googleCalendarMeetConfirmation(eligible, args)
	}

	return s.executePlugin(ctx, userID, eligible, args)
}

func (s *Service) handleGoogleCalendarMeetPending(
	ctx context.Context,
	userID string,
	eligible eligiblePlugin,
	pending *PendingAction,
	text string,
) (Reply, *PendingAction, SessionStatus, error) {
	switch pending.MissingArgument {
	case internalAttendeeEmailKey:
		appendAttendeeEmail(pending.Arguments, text)
		pending.MissingArgument = ""
		return s.advancePlugin(ctx, userID, eligible, pending.Arguments, text)
	case "start_time":
		pending.MissingArgument = ""
		if isRecommendationIntent(text) {
			return s.recommendMeetingSlot(ctx, userID, eligible, pending.Arguments)
		}
		if isExplicitDateTime(text) {
			pending.Arguments["start_time"] = parseExplicitMeetingTime(text)
			return s.advancePlugin(ctx, userID, eligible, pending.Arguments, text)
		}
		pending.MissingArgument = "start_time"
		return Reply{
			Type: ReplyTypeFollowUp,
			Text: scheduleTimePrompt(eligible.catalog.Manifest),
			Action: &ActionInfo{
				PluginSlug: eligible.catalog.Slug,
				Status:     ActionStatusPending,
			},
		}, pending, SessionStatusActive, nil
	default:
		return Reply{}, nil, SessionStatusActive, fmt.Errorf("unexpected google calendar meet pending arg %q", pending.MissingArgument)
	}
}
