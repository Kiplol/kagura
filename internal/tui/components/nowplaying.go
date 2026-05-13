//go:build ignore

package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	barBg     = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	noteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	volStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

// NowPlayingBar renders the full-width status bar shown at the bottom of the screen.
//
//	♪  Aphex Twin – Windowlicker    [=======>.........]  2:14 / 6:51   vol:80
func NowPlayingBar(artist, title string, pos, duration float64, volume int, paused, shuffle, repeat bool, width int) string {
	if title == "" {
		return barBg.Width(width).Render(dimStyle.Render("  ♪  no track loaded"))
	}

	// Left: note + artist – title
	note := noteStyle.Render("♪")
	if paused {
		note = dimStyle.Render("⏸")
	}
	track := titleStyle.Render(truncate(fmt.Sprintf("%s – %s", artist, title), 35))

	// Middle: progress bar + time
	barWidth := 22
	bar := ProgressBar(pos, duration, barWidth)
	elapsed := FormatDuration(pos)
	total := FormatDuration(duration)
	timeStr := dimStyle.Render(fmt.Sprintf("%s / %s", elapsed, total))

	// Right: volume + indicators
	vol := volStyle.Render(fmt.Sprintf("vol:%d", volume))
	var indicators []string
	if shuffle {
		indicators = append(indicators, "🔀")
	}
	if repeat {
		indicators = append(indicators, "🔁")
	}
	indicatorStr := strings.Join(indicators, " ")

	left := fmt.Sprintf("  %s  %s", note, track)
	mid := fmt.Sprintf("  %s  %s", bar, timeStr)
	right := fmt.Sprintf("%s  %s  ", vol, indicatorStr)

	// Pad middle section to fill available space.
	used := lipgloss.Width(left) + lipgloss.Width(mid) + lipgloss.Width(right)
	pad := width - used
	if pad < 1 {
		pad = 1
	}

	line := left + mid + strings.Repeat(" ", pad) + right
	return barBg.Width(width).Render(line)
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
