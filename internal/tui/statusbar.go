package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/rifat977/taplytix/internal/render"
)

type StatusBar struct {
	Alerts      int
	EventsPerSec float64
	SpansActive int
	Connected   bool
	Started     time.Time
	Width       int
}

func NewStatusBar() StatusBar {
	return StatusBar{Started: time.Now(), Connected: true}
}

func (s StatusBar) View() string {
	left := alertSegment(s.Alerts)
	center := fmt.Sprintf("events/s: %.0f · spans: %d", s.EventsPerSec, s.SpansActive)
	right := fmt.Sprintf("%s · uptime %s", connSegment(s.Connected), uptime(s.Started))

	if s.Width <= 0 {
		return render.StatusBar.Render(lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", center, "  ", right))
	}
	return render.StatusBar.Width(s.Width).Render(layoutThree(left, center, right, s.Width-2))
}

func alertSegment(n int) string {
	if n <= 0 {
		return render.StatusOK.Render("● 0 alerts")
	}
	return render.StatusError.Render(fmt.Sprintf("⚠ %d alert(s)", n))
}

func connSegment(ok bool) string {
	if ok {
		return render.StatusOK.Render("● connected")
	}
	return render.StatusError.Render("● disconnected")
}

func uptime(started time.Time) string {
	d := time.Since(started).Round(time.Second)
	return d.String()
}

func layoutThree(left, center, right string, width int) string {
	lw := lipgloss.Width(left)
	cw := lipgloss.Width(center)
	rw := lipgloss.Width(right)
	gap1 := (width - lw - cw - rw) / 2
	gap2 := width - lw - cw - rw - gap1
	if gap1 < 1 {
		gap1 = 1
	}
	if gap2 < 1 {
		gap2 = 1
	}
	return left + spaces(gap1) + center + spaces(gap2) + right
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, n)
	for i := range out {
		out[i] = ' '
	}
	return string(out)
}
