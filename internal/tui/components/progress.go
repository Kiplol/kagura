// Package components contains reusable TUI widgets.
package components

import (
	"fmt"
	"strings"
)

// ProgressBar renders an ASCII progress bar like: [=======>      ]
// width is the total character width including brackets.
func ProgressBar(pos, duration float64, width int) string {
	if width < 4 {
		width = 4
	}
	inner := width - 2 // subtract [ and ]

	var filled int
	if duration > 0 {
		ratio := pos / duration
		if ratio > 1 {
			ratio = 1
		}
		filled = int(ratio * float64(inner))
	}

	// Build bar: filled part ends with '>' unless completely full.
	var bar strings.Builder
	bar.WriteByte('[')
	if filled > 0 {
		bar.WriteString(strings.Repeat("=", filled-1))
		if filled == inner {
			bar.WriteByte('=')
		} else {
			bar.WriteByte('>')
		}
	}
	bar.WriteString(strings.Repeat(" ", inner-filled))
	bar.WriteByte(']')
	return bar.String()
}

// FormatDuration formats a float64 second value as m:ss or h:mm:ss.
func FormatDuration(secs float64) string {
	s := int(secs)
	if s < 0 {
		s = 0
	}
	h := s / 3600
	m := (s % 3600) / 60
	s = s % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
