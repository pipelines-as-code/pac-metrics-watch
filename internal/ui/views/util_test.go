package views

import (
	"testing"
	"time"
)

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "he..."},
		{"hello world", 3, "hel"},
		{"hello world", 2, "he"},
		{"", 5, ""},
		{"ab", 1, "a"},
	}
	for _, tt := range tests {
		got := TruncateStr(tt.s, tt.maxLen)
		if got != tt.want {
			t.Errorf("TruncateStr(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
		}
	}
}

func TestTruncateStrMultibyte(t *testing.T) {
	// Japanese characters are multi-byte in UTF-8
	s := "こんにちは世界" // 7 runes, 21 bytes
	got := TruncateStr(s, 5)
	want := "こん..."
	if got != want {
		t.Errorf("TruncateStr(%q, 5) = %q, want %q", s, got, want)
	}

	// Should not truncate when rune count fits
	got = TruncateStr(s, 7)
	if got != s {
		t.Errorf("TruncateStr(%q, 7) = %q, want %q", s, got, s)
	}
}

func TestRelativeTime(t *testing.T) {
	tests := []struct {
		offset time.Duration
		want   string
	}{
		{30 * time.Second, "30s ago"},
		{5 * time.Minute, "5m ago"},
		{3 * time.Hour, "3h ago"},
		{48 * time.Hour, "2d ago"},
	}
	for _, tt := range tests {
		ref := time.Now().Add(-tt.offset)
		got := RelativeTime(ref)
		if got != tt.want {
			t.Errorf("RelativeTime(-%v) = %q, want %q", tt.offset, got, tt.want)
		}
	}
}

func TestPipelineRunStatusStyle(t *testing.T) {
	// Just verify it doesn't panic for known and unknown statuses
	statuses := []string{"Succeeded", "Failed", "Running", "Unknown", ""}
	for _, s := range statuses {
		style := PipelineRunStatusStyle(s)
		// Render something to ensure the style is usable
		_ = style.Render("test")
	}
}
