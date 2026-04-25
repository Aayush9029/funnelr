package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Aayush9029/funnelr/internal/ports"
	"github.com/Aayush9029/funnelr/internal/state"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Action int

const (
	ActionNone Action = iota
	ActionExpose
	ActionStop
	ActionLogs
)

type Result struct {
	Action Action
	Port   int
}

type Model struct {
	ports       []ports.Port
	active      *state.Session
	cursor      int
	width       int
	customMode  bool
	customValue string
	err         string
	result      Result
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	activeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

func NewModel(open []ports.Port, active *state.Session) Model {
	if len(open) == 0 {
		open = []ports.Port{}
	}
	return Model{ports: open, active: active}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case tea.KeyMsg:
		if m.customMode {
			return m.updateCustom(msg)
		}
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.ports)-1 {
				m.cursor++
			}
		case " ", "space", "enter", "return":
			if len(m.ports) > 0 {
				m.result = Result{Action: ActionExpose, Port: m.ports[m.cursor].Number}
				return m, tea.Quit
			}
		case "c":
			m.customMode = true
			m.customValue = ""
			m.err = ""
		case "s":
			m.result = Result{Action: ActionStop}
			return m, tea.Quit
		case "l":
			port := 0
			if len(m.ports) > 0 {
				port = m.ports[m.cursor].Number
			}
			if m.active != nil && port == 0 {
				port = m.active.TargetPort
			}
			if port > 0 {
				m.result = Result{Action: ActionLogs, Port: port}
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m Model) updateCustom(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.customMode = false
		m.customValue = ""
		m.err = ""
	case "enter":
		port, err := strconv.Atoi(strings.TrimSpace(m.customValue))
		if err != nil || port <= 0 || port > 65535 {
			m.err = "enter a port between 1 and 65535"
			return m, nil
		}
		m.result = Result{Action: ActionExpose, Port: port}
		return m, tea.Quit
	case "backspace", "ctrl+h":
		if len(m.customValue) > 0 {
			m.customValue = m.customValue[:len(m.customValue)-1]
		}
	default:
		s := msg.String()
		if len(s) == 1 && s[0] >= '0' && s[0] <= '9' && len(m.customValue) < 5 {
			m.customValue += s
		}
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("funnelr"))
	b.WriteString("\n")
	if m.active != nil {
		b.WriteString(activeStyle.Render(fmt.Sprintf("● active: localhost:%d → %s", m.active.TargetPort, m.active.URL)))
		b.WriteString("\n")
	} else {
		b.WriteString(dimStyle.Render("no active tunnel"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if m.customMode {
		b.WriteString("custom port: ")
		b.WriteString(m.customValue)
		b.WriteString("█\n")
		if m.err != "" {
			b.WriteString(errorStyle.Render(m.err))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("enter expose  esc cancel"))
		b.WriteString("\n")
		return b.String()
	}

	if len(m.ports) == 0 {
		b.WriteString(dimStyle.Render("no common local web ports are open"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("press c to enter a custom port"))
		b.WriteString("\n")
	} else {
		for i, p := range m.ports {
			cursor := " "
			if i == m.cursor {
				cursor = "›"
			}
			circle := "○"
			lineStyle := lipgloss.NewStyle()
			if m.active != nil && m.active.TargetPort == p.Number {
				circle = "●"
				lineStyle = activeStyle
			}
			b.WriteString(lineStyle.Render(fmt.Sprintf("%s %s localhost:%d", cursor, circle, p.Number)))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("↑/↓ move  space expose  c custom  s stop  l logs  q quit"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) Result() Result {
	return m.result
}
