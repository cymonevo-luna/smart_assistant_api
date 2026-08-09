package builtin

import (
	"testing"
	"time"
)

func TestParseDateTimeRFC3339(t *testing.T) {
	got, err := ParseDateTime("2026-08-09T14:00:00Z", time.UTC)
	if err != nil {
		t.Fatalf("ParseDateTime: %v", err)
	}
	if got.UTC().Hour() != 14 {
		t.Fatalf("hour = %d", got.Hour())
	}
}

func TestParseDateTimeAfternoonVariants(t *testing.T) {
	now := time.Now().UTC()
	for _, input := range []string{"this afternoon", "afternoon", "This afternoon"} {
		got, err := ParseDateTime(input, time.UTC)
		if err != nil {
			t.Fatalf("ParseDateTime(%q): %v", input, err)
		}
		if got.UTC().Hour() != 14 {
			t.Fatalf("ParseDateTime(%q) hour = %d, want 14", input, got.Hour())
		}
		if !got.After(now) {
			t.Fatalf("ParseDateTime(%q) not in future: %v", input, got)
		}
	}
}

func TestParseDateTimeTomorrowMorning(t *testing.T) {
	got, err := ParseDateTime("tomorrow morning", time.UTC)
	if err != nil {
		t.Fatalf("ParseDateTime: %v", err)
	}
	tomorrow := time.Now().UTC().Add(24 * time.Hour)
	if got.UTC().Day() != tomorrow.Day() || got.UTC().Month() != tomorrow.Month() {
		t.Fatalf("expected tomorrow, got %v", got)
	}
	if got.UTC().Hour() != 9 {
		t.Fatalf("hour = %d, want 9", got.Hour())
	}
}

func TestParseDateTimeClockShorthand(t *testing.T) {
	now := time.Now().UTC()
	for _, input := range []string{"3pm", "3pm today", "3:30pm", "15:00"} {
		got, err := ParseDateTime(input, time.UTC)
		if err != nil {
			t.Fatalf("ParseDateTime(%q): %v", input, err)
		}
		if !got.After(now) {
			t.Fatalf("ParseDateTime(%q) not in future: %v", input, got)
		}
	}

	got, err := ParseDateTime("3pm", time.UTC)
	if err != nil {
		t.Fatalf("ParseDateTime(3pm): %v", err)
	}
	if got.UTC().Hour() != 15 {
		t.Fatalf("3pm hour = %d, want 15", got.Hour())
	}
}

func TestParseDateTimeCombined(t *testing.T) {
	now := time.Now().UTC()
	for _, input := range []string{"tomorrow afternoon", "this afternoon", "today at 3pm"} {
		got, err := ParseDateTime(input, time.UTC)
		if err != nil {
			t.Fatalf("ParseDateTime(%q): %v", input, err)
		}
		if !got.After(now) {
			t.Fatalf("ParseDateTime(%q) not in future: %v", input, got)
		}
	}
}

func TestParseDateTimeUnparseable(t *testing.T) {
	_, err := ParseDateTime("sometime maybe", time.UTC)
	if err == nil {
		t.Fatal("expected error for unparseable time")
	}
}

func TestParseDateTimePeriodDefaults(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		input string
		hour  int
	}{
		{"morning", 9},
		{"evening", 18},
		{"tonight", 18},
		{"noon", 12},
	}
	for _, tc := range cases {
		got, err := ParseDateTime(tc.input, time.UTC)
		if err != nil {
			t.Fatalf("ParseDateTime(%q): %v", tc.input, err)
		}
		if got.UTC().Hour() != tc.hour {
			t.Fatalf("ParseDateTime(%q) hour = %d, want %d", tc.input, got.Hour(), tc.hour)
		}
		if !got.After(now) {
			t.Fatalf("ParseDateTime(%q) not in future", tc.input)
		}
	}
}
