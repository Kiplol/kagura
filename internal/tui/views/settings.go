//go:build ignore

package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kip/kagura/internal/config"
)

// HotkeysSavedMsg is sent when the user saves updated hotkeys.
type HotkeysSavedMsg struct{ Hotkeys config.Hotkeys }

// action is a single mappable action entry.
type action struct {
	name    string
	label   string
	current *string // pointer into config.Hotkeys
}

// SettingsModel is the hotkey remapping screen.
type SettingsModel struct {
	hotkeys  config.Hotkeys
	actions  []action
	cursor   int
	binding  bool // true when waiting for a new key combo
	width    int
	height   int
	message  string
}

// NewSettings creates a SettingsModel pre-loaded with current hotkeys.
func NewSettings(hotkeys config.Hotkeys, width, height int) SettingsModel {
	m := SettingsModel{
		hotkeys: hotkeys,
		width:   width,
		height:  height,
	}
	m.buildActions()
	return m
}

func (m *SettingsModel) buildActions() {
	m.actions = []action{
		{name: "play_pause", label: "Play / Pause", current: &m.hotkeys.PlayPause},
		{name: "next", label: "Next Track", current: &m.hotkeys.Next},
		{name: "prev", label: "Previous Track", current: &m.hotkeys.Prev},
		{name: "volume_up", label: "Volume Up", current: &m.hotkeys.VolumeUp},
		{name: "volume_down", label: "Volume Down", current: &m.hotkeys.VolumeDown},
		{name: "toggle_mute", label: "Toggle Mute", current: &m.hotkeys.ToggleMute},
	}
}

func (m SettingsModel) Init() tea.Cmd { return nil }

func (m SettingsModel) Update(msg tea.Msg) (SettingsModel, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tea.KeyMsg:
		if m.binding {
			return m.captureBinding(msg)
		}
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.actions)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter":
			m.binding = true
			m.message = "Press a key combo to bind to \"" + m.actions[m.cursor].label + "\" -- or Esc to cancel"
		case "r":
			defaults := config.Defaults().Hotkeys
			m.hotkeys = defaults
			m.buildActions()
			m.message = "All bindings reset to defaults"
			return m, m.save()
		case "esc", "s", "q":
			return m, func() tea.Msg { return HotkeysSavedMsg{Hotkeys: m.hotkeys} }
		}
	}
	return m, nil
}

func (m SettingsModel) captureBinding(msg tea.KeyMsg) (SettingsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.binding = false
		m.message = "Cancelled"
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	default:
		combo := msg.String()
		*m.actions[m.cursor].current = combo
		m.binding = false
		m.message = fmt.Sprintf("Bound \"%s\" to %s", combo, m.actions[m.cursor].label)
		return m, m.save()
	}
}

func (m SettingsModel) save() tea.Cmd {
	h := m.hotkeys
	return func() tea.Msg { return HotkeysSavedMsg{Hotkeys: h} }
}

func (m SettingsModel) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	labelStyle := lipgloss.NewStyle().Width(20)
	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)
	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("255"))
	msgStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Italic(true)

	var rows []string
	for i, a := range m.actions {
		label := labelStyle.Render(a.label)
		key := keyStyle.Render(*a.current)
		row := fmt.Sprintf("  %s  %s", label, key)
		if i == m.cursor {
			row = selectedStyle.Render(fmt.Sprintf("▶ %s  %s", label, key))
		}
		rows = append(rows, row)
	}

	hints := []string{
		"j/k or ↑/↓ to move",
		"enter to rebind",
		"r to reset all defaults",
		"esc to close",
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("⌨  Global Hotkeys"),
		"",
		strings.Join(rows, "\n"),
		"",
		hintStyle.Render(strings.Join(hints, "  •  ")),
		"",
		msgStyle.Render(m.message),
	)

	if m.binding {
		overlay := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205")).
			Padding(1, 3).
			Render(msgStyle.Render(m.message))
		content = lipgloss.JoinVertical(lipgloss.Left, content, "", overlay)
	}

	return lipgloss.NewStyle().Padding(1, 2).Render(content)
}
