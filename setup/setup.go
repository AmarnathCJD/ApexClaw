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

// ─── Styles ───────────────────────────────────────────────────────────────────

var (
	focusedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	blurredStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	checkedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
)

// ─── Interface definitions ─────────────────────────────────────────────────────

type interfaceDef struct {
	id     string
	label  string
	emoji  string
	fields []fieldDef
}

type fieldDef struct {
	key         string
	label       string
	placeholder string
	required    bool
	secret      bool
	numericOnly bool
}

var sharedFields = []fieldDef{
	{key: "DEFAULT_MODEL", label: "Default AI Model", placeholder: "GLM-4.7"},
	{key: "MAX_ITERATIONS", label: "Max Agent Iterations", placeholder: "10", numericOnly: true},
	{key: "DNS", label: "Custom DNS Server", placeholder: "1.1.1.1 (optional)"},
	{key: "ZAI_TOKEN", label: "ZAI Token", placeholder: "your-token-here", secret: true},
	{key: "EMAIL_ADDRESS", label: "Gmail Address", placeholder: "your.email@gmail.com"},
	{key: "EMAIL_PASSWORD", label: "Gmail App Password", placeholder: "16-character app password", secret: true},
}

var allInterfaces = []interfaceDef{
	{
		id:    "telegram",
		label: "Telegram Bot",
		emoji: "✈️",
		fields: []fieldDef{
			{key: "TELEGRAM_API_ID", label: "Telegram API ID", placeholder: "123456789", numericOnly: true},
			{key: "TELEGRAM_API_HASH", label: "Telegram API Hash", placeholder: "abcdef123456...", secret: true},
			{key: "TELEGRAM_BOT_TOKEN", label: "Telegram Bot Token", placeholder: "123456:ABC-DEF...", secret: true},
			{key: "OWNER_ID", label: "Owner Telegram Chat ID", placeholder: "123456789", numericOnly: true},
		},
	},
	{
		id:    "whatsapp",
		label: "WhatsApp Bot",
		emoji: "📱",
		fields: []fieldDef{
			{key: "WA_OWNER_ID", label: "Your WhatsApp Number (intl format)", placeholder: "919876543210"},
		},
	},
	{
		id:    "web",
		label: "Web UI",
		emoji: "🌐",
		fields: []fieldDef{
			{key: "WEB_PORT", label: "Web Server Port", placeholder: ":8080"},
			{key: "WEB_LOGIN_CODE", label: "Web Login Code (6 digits)", placeholder: "123456", secret: true},
		},
	},
	{
		id:    "nvidia",
		label: "NVIDIA NIM",
		emoji: "🟢",
		fields: []fieldDef{
			{key: "NVIDIA_API_KEY", label: "NVIDIA API Key", placeholder: "nvapi-...", required: true, secret: true},
		},
	},
	{
		id:    "openrouter",
		label: "OpenRouter",
		emoji: "🔀",
		fields: []fieldDef{
			{key: "OPENROUTER_API_KEY", label: "OpenRouter API Key", placeholder: "sk-or-...", required: true, secret: true},
		},
	},
	{
		id:    "groq",
		label: "Groq",
		emoji: "⚡",
		fields: []fieldDef{
			{key: "GROQ_API_KEY", label: "Groq API Key", placeholder: "gsk_...", required: true, secret: true},
		},
	},
}

// ─── Phase 1: Interface Selector ──────────────────────────────────────────────

type SelectorModel struct {
	interfaces []interfaceDef
	selected   map[string]bool
	cursor     int
	done       bool
}

func NewSelectorModel() *SelectorModel {
	selected := make(map[string]bool)
	// Pre-tick based on what's already configured
	for _, iface := range allInterfaces {
		for _, f := range iface.fields {
			if os.Getenv(f.key) != "" {
				selected[iface.id] = true
				break
			}
		}
	}
	// Web is on by default
	if _, ok := selected["web"]; !ok {
		selected["web"] = true
	}
	return &SelectorModel{
		interfaces: allInterfaces,
		selected:   selected,
	}
}

func (m *SelectorModel) Init() tea.Cmd { return nil }

func (m *SelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.interfaces)-1 {
				m.cursor++
			}
		case " ":
			id := m.interfaces[m.cursor].id
			m.selected[id] = !m.selected[id]
		case "enter":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *SelectorModel) View() string {
	var s strings.Builder

	s.WriteString(titleStyle.Render("🐾 ApexClaw Setup Wizard") + "\n\n")
	s.WriteString(sectionStyle.Render("Which interfaces do you want to configure?") + "\n")
	s.WriteString(helpStyle.Render("  Space: toggle  |  ↑/↓: navigate  |  Enter: confirm") + "\n\n")

	for i, iface := range m.interfaces {
		cursor := "  "
		if i == m.cursor {
			cursor = focusedStyle.Render("→ ")
		}

		checkbox := "[ ]"
		label := blurredStyle.Render(iface.emoji + " " + iface.label)
		if m.selected[iface.id] {
			checkbox = checkedStyle.Render("[✓]")
			label = iface.emoji + " " + iface.label
		}

		s.WriteString(fmt.Sprintf("%s%s %s\n", cursor, checkbox, label))
	}

	s.WriteString("\n")
	s.WriteString(helpStyle.Render("Web UI is always available regardless of selection."))

	return s.String()
}

// ─── Phase 2: Field Entry ──────────────────────────────────────────────────────

type FieldModel struct {
	currentField int
	fields       []*Field
	err          error
	submitted    bool
}

type Field struct {
	key         string
	label       string
	placeholder string
	input       textinput.Model
	value       string
	required    bool
	secret      bool
	numericOnly bool
	section     string // the interface this field belongs to (empty = shared)
}

type SubmitMsg struct{}
type ErrorMsg error

func NewFieldModel(selectedIfaces map[string]bool) *FieldModel {
	var fieldDefs []struct {
		section string
		def     fieldDef
	}

	for _, iface := range allInterfaces {
		if !selectedIfaces[iface.id] {
			continue
		}
		for _, f := range iface.fields {
			fieldDefs = append(fieldDefs, struct {
				section string
				def     fieldDef
			}{iface.label, f})
		}
	}

	// Always add shared fields
	for _, f := range sharedFields {
		fieldDefs = append(fieldDefs, struct {
			section string
			def     fieldDef
		}{"General", f})
	}

	m := &FieldModel{}
	for _, fd := range fieldDefs {
		ti := textinput.New()
		ti.Placeholder = fd.def.placeholder
		if fd.def.secret {
			ti.EchoMode = textinput.EchoPassword
		}
		val := os.Getenv(fd.def.key)
		field := &Field{
			key:         fd.def.key,
			label:       fd.def.label,
			placeholder: fd.def.placeholder,
			input:       ti,
			value:       val,
			required:    fd.def.required,
			secret:      fd.def.secret,
			numericOnly: fd.def.numericOnly,
			section:     fd.section,
		}
		if val != "" {
			field.input.SetValue(val)
		}
		m.fields = append(m.fields, field)
	}

	if len(m.fields) > 0 {
		m.fields[0].input.Focus()
	}
	return m
}

func (m *FieldModel) Init() tea.Cmd { return textinput.Blink }

func (m *FieldModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m *FieldModel) View() string {
	var s strings.Builder

	s.WriteString(titleStyle.Render("🐾 ApexClaw Setup Wizard") + "\n\n")

	if m.err != nil {
		s.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("⚠  %v\n\n", m.err)))
	}

	progress := fmt.Sprintf("Field %d of %d", m.currentField+1, len(m.fields))
	s.WriteString(helpStyle.Render(progress) + "\n\n")

	lastSection := ""
	for i, field := range m.fields {
		if field.section != lastSection {
			s.WriteString(sectionStyle.Render("── "+field.section+" ──") + "\n")
			lastSection = field.section
		}

		if i == m.currentField {
			s.WriteString(focusedStyle.Render("→ " + field.label))
			s.WriteString("\n")
			if field.secret && field.input.Value() != "" {
				s.WriteString(blurredStyle.Render("  " + strings.Repeat("•", 8)))
			} else {
				s.WriteString("  " + field.input.View())
			}
			s.WriteString("\n\n")
		} else {
			if field.value != "" {
				s.WriteString(checkedStyle.Render("✓ ") + field.label + "\n")
			} else if field.required {
				s.WriteString(blurredStyle.Render("○ "+field.label) + "\n")
			} else {
				s.WriteString(blurredStyle.Render("◌ "+field.label+" (optional)") + "\n")
			}
		}
	}

	s.WriteString("\n")
	s.WriteString(helpStyle.Render("↑/↓ or Tab: Navigate  |  Enter: Next  |  Ctrl+C: Quit"))
	return s.String()
}

func (m *FieldModel) nextField() {
	m.currentField = (m.currentField + 1) % len(m.fields)
	m.fields[m.currentField].input.Focus()
}

func (m *FieldModel) prevField() {
	if m.currentField > 0 {
		m.currentField--
	} else {
		m.currentField = len(m.fields) - 1
	}
	m.fields[m.currentField].input.Focus()
}

func (m *FieldModel) submit() tea.Cmd {
	return func() tea.Msg {
		for _, field := range m.fields {
			if field.required && field.value == "" {
				return ErrorMsg(fmt.Errorf("'%s' is required", field.label))
			}
			if field.numericOnly && field.value != "" {
				if _, err := strconv.Atoi(field.value); err != nil {
					return ErrorMsg(fmt.Errorf("'%s' must be numeric", field.label))
				}
			}
		}
		if err := saveFieldsToEnv(m.fields); err != nil {
			return ErrorMsg(err)
		}
		return SubmitMsg{}
	}
}

// ─── Persistence ──────────────────────────────────────────────────────────────

func saveFieldsToEnv(fields []*Field) error {
	_ = godotenv.Load()
	envMap, _ := godotenv.Read()
	if envMap == nil {
		envMap = make(map[string]string)
	}
	for _, field := range fields {
		if field.value != "" {
			os.Setenv(field.key, field.value)
			envMap[field.key] = field.value
		}
	}
	return godotenv.Write(envMap, ".env")
}

// ─── Entry point ──────────────────────────────────────────────────────────────

func InteractiveSetup() error {
	// Skip if any interface is fully configured
	configured := false
	for _, iface := range allInterfaces {
		allSet := true
		for _, f := range iface.fields {
			if f.required && os.Getenv(f.key) == "" {
				allSet = false
				break
			}
		}
		if allSet && len(iface.fields) > 0 {
			configured = true
			break
		}
	}
	// Also skip if Telegram fully set (legacy check)
	tgKeys := []string{"TELEGRAM_API_ID", "TELEGRAM_API_HASH", "TELEGRAM_BOT_TOKEN", "OWNER_ID"}
	tgOk := true
	for _, k := range tgKeys {
		if os.Getenv(k) == "" {
			tgOk = false
			break
		}
	}
	if tgOk {
		configured = true
	}

	if configured {
		return nil
	}

	fmt.Print("\n🔧 Run ApexClaw configuration wizard? (y/n): ")
	var response string
	fmt.Scanln(&response)
	if strings.ToLower(strings.TrimSpace(response)) != "y" {
		return nil
	}

	// Phase 1: Interface selector
	sel := NewSelectorModel()
	p1 := tea.NewProgram(sel)
	raw, err := p1.Run()
	if err != nil {
		return fmt.Errorf("setup wizard failed: %w", err)
	}
	selResult := raw.(*SelectorModel)
	if !selResult.done {
		return fmt.Errorf("setup was cancelled")
	}

	// Phase 2: Field entry
	fm := NewFieldModel(selResult.selected)
	if len(fm.fields) == 0 {
		return nil
	}
	p2 := tea.NewProgram(fm)
	raw2, err := p2.Run()
	if err != nil {
		return fmt.Errorf("setup wizard failed: %w", err)
	}
	fmResult := raw2.(*FieldModel)
	if !fmResult.submitted {
		return fmt.Errorf("setup was cancelled")
	}

	return nil
}
