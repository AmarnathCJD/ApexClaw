package setup

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joho/godotenv"
)

// Model holds the state of the setup wizard
type Model struct {
	currentField int
	fields       []*Field
	focused      bool
	submitted    bool
	err          error
}

// Field represents a configuration field
type Field struct {
	key         string
	label       string
	placeholder string
	input       textinput.Model
	value       string
	required    bool
	secret      bool
}

var (
	focusedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205"))
	blurredStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212"))
	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
)

func NewSetup() *Model {
	m := &Model{
		currentField: 0,
		fields:       make([]*Field, 0),
	}

	fields := []struct {
		key         string
		label       string
		placeholder string
		required    bool
		secret      bool
	}{
		{
			key:         "TELEGRAM_API_ID",
			label:       "Telegram API ID",
			placeholder: "123456789",
			required:    false,
			secret:      false,
		},
		{
			key:         "TELEGRAM_API_HASH",
			label:       "Telegram API Hash",
			placeholder: "abcdef123456...",
			required:    false,
			secret:      true,
		},
		{
			key:         "TELEGRAM_BOT_TOKEN",
			label:       "Telegram Bot Token",
			placeholder: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
			required:    false,
			secret:      true,
		},
		{
			key:         "OWNER_ID",
			label:       "Owner ID (Your Telegram Chat ID)",
			placeholder: "123456789",
			required:    false,
			secret:      false,
		},
		{
			key:         "ZAI_TOKEN",
			label:       "ZAI Token",
			placeholder: "your-token-here",
			required:    false,
			secret:      true,
		},
		{
			key:         "EMAIL_ADDRESS",
			label:       "Gmail Address",
			placeholder: "your.email@gmail.com",
			required:    false,
			secret:      false,
		},
		{
			key:         "EMAIL_PASSWORD",
			label:       "Gmail App Password",
			placeholder: "16-character app password",
			required:    false,
			secret:      true,
		},
	}

	for _, f := range fields {
		ti := textinput.New()
		ti.Placeholder = f.placeholder
		if f.secret {
			ti.EchoMode = textinput.EchoPassword
		}

		value := os.Getenv(f.key)

		field := &Field{
			key:         f.key,
			label:       f.label,
			placeholder: f.placeholder,
			input:       ti,
			value:       value,
			required:    f.required,
			secret:      f.secret,
		}
		if value != "" {
			field.input.SetValue(value)
		}
		m.fields = append(m.fields, field)
	}

	if len(m.fields) > 0 {
		m.fields[0].input.Focus()
	}

	return m
}

func (m *Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "tab", "down":
			m.nextField()
			return m, nil
		case "shift+tab", "up":
			m.prevField()
			return m, nil
		case "enter":
			if m.currentField == len(m.fields)-1 {
				return m, m.submit()
			}
			m.nextField()
			return m, nil
		}
	case SubmitMsg:
		m.submitted = true
		return m, tea.Quit
	case ErrorMsg:
		m.err = msg
		return m, nil
	}

	if m.currentField < len(m.fields) {
		m.fields[m.currentField].input.Focus()
		field := m.fields[m.currentField]
		newInput, cmd := field.input.Update(msg)
		field.input = newInput
		field.value = field.input.Value()
		return m, cmd
	}

	return m, nil
}

func (m *Model) View() string {
	var s strings.Builder

	s.WriteString(titleStyle.Render("🐾 ApexClaw Setup Wizard") + "\n\n")

	if m.err != nil {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("Error: %v\n\n", m.err)))
	}

	progress := fmt.Sprintf("Step %d of %d", m.currentField+1, len(m.fields))
	s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(progress) + "\n\n")

	for i, field := range m.fields {
		if i == m.currentField {
			s.WriteString(focusedStyle.Render("→ " + field.label))
			s.WriteString("\n")
			if field.secret && field.input.Value() != "" {
				s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  " + strings.Repeat("•", 8)))
			} else {
				s.WriteString("  " + field.input.View())
			}
			s.WriteString("\n")
		} else {
			if field.value != "" {
				mark := "✓"
				if field.required {
					s.WriteString(focusedStyle.Render(mark))
				} else {
					s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(mark))
				}
				s.WriteString(" " + field.label + "\n")
			} else if field.required {
				s.WriteString(blurredStyle.Render("○ " + field.label + "\n"))
			} else {
				s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("◌ " + field.label + " (optional)\n"))
			}
		}
		s.WriteString("\n")
	}

	s.WriteString("\n")
	s.WriteString(helpStyle.Render("↑/↓ or Shift+Tab/Tab: Navigate | Enter: Next | Ctrl+C: Quit"))

	return s.String()
}

func (m *Model) nextField() {
	m.currentField = (m.currentField + 1) % len(m.fields)
	m.fields[m.currentField].input.Focus()
}

func (m *Model) prevField() {
	if m.currentField > 0 {
		m.currentField--
	} else {
		m.currentField = len(m.fields) - 1
	}
	m.fields[m.currentField].input.Focus()
}

func (m *Model) submit() tea.Cmd {
	return func() tea.Msg {
		for _, field := range m.fields {
			if field.required && field.value == "" {
				return ErrorMsg(fmt.Errorf("'%s' is required", field.label))
			}
			if field.value != "" && (field.key == "TELEGRAM_API_ID" || field.key == "OWNER_ID") {
				if _, err := strconv.Atoi(field.value); err != nil {
					return ErrorMsg(fmt.Errorf("'%s' must be numeric", field.label))
				}
			}
		}

		if err := saveToEnv(m.fields); err != nil {
			return ErrorMsg(err)
		}

		return SubmitMsg{}
	}
}

type SubmitMsg struct{}

type ErrorMsg error

func saveToEnv(fields []*Field) error {
	_ = godotenv.Load()
	for _, field := range fields {
		if field.value != "" {
			os.Setenv(field.key, field.value)
		}
	}

	f, err := os.OpenFile(".env", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open .env file: %v", err)
	}
	defer f.Close()

	for _, field := range fields {
		if field.value != "" {
			if _, err := fmt.Fprintf(f, "%s=%s\n", field.key, field.value); err != nil {
				return fmt.Errorf("failed to write to .env: %v", err)
			}
		}
	}

	return nil
}

func InteractiveSetup() error {
	// Only show setup on first run (no Telegram credentials set)
	requiredVars := []string{"TELEGRAM_API_ID", "TELEGRAM_API_HASH", "TELEGRAM_BOT_TOKEN", "OWNER_ID"}
	hasSetup := true
	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			hasSetup = false
			break
		}
	}

	if hasSetup {
		return nil // Already configured, skip setup
	}

	fmt.Print("\n🔧 Run configuration setup? (y/n): ")
	var response string
	fmt.Scanln(&response)

	if strings.ToLower(strings.TrimSpace(response)) != "y" {
		return nil // User skipped setup
	}

	m := NewSetup()
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("setup wizard failed: %v", err)
	}

	model := finalModel.(*Model)
	if !model.submitted {
		return fmt.Errorf("setup was cancelled")
	}

	return nil
}
