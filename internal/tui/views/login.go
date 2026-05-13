// Package views contains full-screen TUI views for the application.
package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LoginDoneMsg is emitted when the user submits valid credentials.
type LoginDoneMsg struct {
	URL      string
	Username string
	Password string
}

// LoginErrMsg is emitted when a connection attempt fails.
type LoginErrMsg struct{ Err error }

// loginField is an index into the form fields.
type loginField int

const (
	fieldURL loginField = iota
	fieldUsername
	fieldPassword
	fieldCount
)

var loginLabels = [fieldCount]string{"Server URL", "Username", "Password"}

// LoginModel is the first-run / login view.
type LoginModel struct {
	inputs  [fieldCount]textinput.Model
	focused loginField
	err     string
	loading bool
	width   int
	height  int
}

// NewLogin creates a fresh LoginModel with optional pre-filled URL.
func NewLogin(prefillURL string) LoginModel {
	var inputs [fieldCount]textinput.Model
	for i := range inputs {
		t := textinput.New()
		t.CharLimit = 256
		inputs[i] = t
	}

	inputs[fieldURL].Placeholder = "http://localhost:4533"
	inputs[fieldURL].SetValue(prefillURL)
	inputs[fieldUsername].Placeholder = "admin"
	inputs[fieldPassword].Placeholder = "••••••••"
	inputs[fieldPassword].EchoMode = textinput.EchoPassword
	inputs[fieldPassword].EchoCharacter = '•'

	inputs[fieldURL].Focus()

	return LoginModel{inputs: inputs, focused: fieldURL}
}

func (m LoginModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m LoginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "tab", "down", "enter":
			if msg.String() == "enter" && m.focused == fieldPassword {
				// Submit
				return m, m.submit()
			}
			m.inputs[m.focused].Blur()
			m.focused = (m.focused + 1) % fieldCount
			m.inputs[m.focused].Focus()
			return m, textinput.Blink

		case "shift+tab", "up":
			m.inputs[m.focused].Blur()
			m.focused = (m.focused + fieldCount - 1) % fieldCount
			m.inputs[m.focused].Focus()
			return m, textinput.Blink
		}

	case LoginErrMsg:
		m.loading = false
		m.err = msg.Err.Error()
		return m, nil
	}

	// Forward keystrokes to the focused input.
	var cmd tea.Cmd
	m.inputs[m.focused], cmd = m.inputs[m.focused].Update(msg)
	return m, cmd
}

func (m LoginModel) submit() tea.Cmd {
	return func() tea.Msg {
		return LoginDoneMsg{
			URL:      strings.TrimRight(m.inputs[fieldURL].Value(), "/"),
			Username: m.inputs[fieldUsername].Value(),
			Password: m.inputs[fieldPassword].Value(),
		}
	}
}

// View renders the login form.
func (m LoginModel) View() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Render("♪  navidrome-tui")

	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("Connect to your Navidrome server")

	var fields []string
	labelStyle := lipgloss.NewStyle().Width(12).Foreground(lipgloss.Color("244"))
	for i := loginField(0); i < fieldCount; i++ {
		label := labelStyle.Render(loginLabels[i])
		input := m.inputs[i].View()
		fields = append(fields, fmt.Sprintf("%s  %s", label, input))
	}

	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).
		Render("tab / shift+tab to move  •  enter to connect")

	var errLine string
	if m.err != "" {
		errLine = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).
			Render("✗  " + m.err)
	}

	var status string
	if m.loading {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).
			Render("Connecting…")
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 3).
		Width(54).
		Render(lipgloss.JoinVertical(lipgloss.Left,
			title,
			"",
			subtitle,
			"",
			strings.Join(fields, "\n"),
			"",
			hint,
			errLine,
			status,
		))

	// Center on screen.
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
