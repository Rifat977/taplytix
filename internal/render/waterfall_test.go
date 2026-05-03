package render

import (
	"strings"
	"testing"
	"time"

	"github.com/rifat977/taplytix/internal/model"
)

func TestWaterfallRowFullWidthBar(t *testing.T) {
	start := time.Unix(0, 0)
	span := &model.SpanEvent{
		Name: "GET /api", StartTime: start, Duration: 100 * time.Millisecond, Status: model.StatusOK,
	}
	got := WaterfallRow(span, start, 100*time.Millisecond, "", 80)
	plain := stripANSI(got)
	if !strings.Contains(plain, "GET /api") {
		t.Errorf("missing span name in row: %q", plain)
	}
	if !strings.Contains(plain, "100ms") {
		t.Errorf("missing duration label in row: %q", plain)
	}
	if !strings.Contains(plain, "█") {
		t.Errorf("expected at least one full block in bar: %q", plain)
	}
}

func TestWaterfallRowOffsetReflectsStart(t *testing.T) {
	start := time.Unix(0, 0)
	child := &model.SpanEvent{
		Name: "child", StartTime: start.Add(50 * time.Millisecond),
		Duration: 10 * time.Millisecond, Status: model.StatusOK,
	}
	got := stripANSI(WaterfallRow(child, start, 100*time.Millisecond, "", 80))
	// Bar should be roughly halfway across — there must be substantial leading
	// whitespace before any block char.
	idx := strings.IndexAny(got, "▏▎▍▌▋▊▉█")
	if idx < 0 {
		t.Fatalf("no bar found in %q", got)
	}
	// Strip the indent prefix consideration; the leading space count between
	// the name+space and the first block should be > 10 cells.
	if idx < 30 {
		t.Errorf("bar started too early at idx=%d in %q", idx, got)
	}
}

func TestSpanColorMatchesStatusAndDuration(t *testing.T) {
	cases := []struct {
		name string
		s    model.SpanEvent
		want string
	}{
		{"ok-fast", model.SpanEvent{Status: model.StatusOK, Duration: 10 * time.Millisecond}, string(ColorSuccess)},
		{"ok-slow", model.SpanEvent{Status: model.StatusOK, Duration: 300 * time.Millisecond}, string(ColorWarning)},
		{"error", model.SpanEvent{Status: model.StatusError, Duration: 5 * time.Millisecond}, string(ColorDanger)},
	}
	for _, tc := range cases {
		s := tc.s
		if got := string(spanColor(&s)); got != tc.want {
			t.Errorf("%s: spanColor = %s, want %s", tc.name, got, tc.want)
		}
	}
}
