//go:build ignore

package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	pinkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("218")) // soft pink
	redStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("160")) // red
)

// BongoCatFrame returns the coloured ASCII frame for the given beat phase (0 or 1).
func BongoCatFrame(phase int) string {
	var frame string
	if phase%2 == 0 {
		frame = frameIdle
	} else {
		frame = frameBeat
	}
	// Colour [oo] pink (paw pads) and ii/i red (impact ticks).
	frame = strings.ReplaceAll(frame, "[oo]", pinkStyle.Render("(oo)"))
	frame = strings.ReplaceAll(frame, "ii", redStyle.Render("ii"))
	frame = strings.ReplaceAll(frame, " i\n", redStyle.Render(" i")+"\n")
	return frame
}

// BongoCatWidth is the character width of each frame.
const BongoCatWidth = 34

// Bongo cat: wide horizontal blob, snout tapers left, two pointy ears top-right.
// The left paw raises and lowers on alternating beats. Red ticks mark impact.
// [oo] = pink paw pad, ii/i = red impact ticks (coloured at render time).

// frameIdle: left paw resting down (just struck), impact marks bottom-left.
const frameIdle = `
                       /\  /\
  ___________________ /  \/  \
 / ●                           \
|  ~~  .          [oo]          |
 \_______________________________/
 ii
  i`

// frameBeat: left paw raised high (about to strike), marks bottom-right.
const frameBeat = `
 [oo]
  |    _________________  /\  /\
  \___/ ●              _ /  \/  \
       |  ~~  .       [oo]       |
        \_________________________/
                              ii
                               i`
