package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Aayush9029/funnelr/internal/funnel"
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

type ExposeFunc func(context.Context, int, funnel.ProgressFunc) (funnel.ExposeResult, error)
type StopFunc func(context.Context) error

type Model struct {
	ports       []ports.Port
	active      *state.Session
	traffic     state.Traffic
	expose      ExposeFunc
	stop        StopFunc
	cursor      int
	width       int
	frame       int
	customMode  bool
	customValue string
	err         string
	status      string
	exposing    bool
	result      Result
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	activeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

func NewModel(open []ports.Port, active *state.Session, expose ExposeFunc, stop StopFunc) Model {
	if len(open) == 0 {
		open = []ports.Port{}
	}
	model := Model{ports: open, active: active, expose: expose, stop: stop}
	model.refreshTraffic()
	return model
}

func (m Model) Init() tea.Cmd { return tickCmd() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case tea.KeyMsg:
		if m.exposing {
			switch msg.String() {
			case "ctrl+c", "q", "esc":
				return m, tea.Quit
			}
			return m, nil
		}
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
				return m.startExpose(m.ports[m.cursor].Number)
			}
		case "c":
			m.customMode = true
			m.customValue = ""
			m.err = ""
		case "s":
			if m.stop == nil {
				m.err = "stop is unavailable"
				return m, nil
			}
			m.status = "stopping funnelr..."
			return m, stopCmd(m.stop)
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
			m.err = "no log file yet"
		}
	case exposeDoneMsg:
		m.exposing = false
		if msg.Err != nil {
			m.err = msg.Err.Error()
			m.status = ""
			return m, nil
		}
		m.active = &msg.Result.Session
		m.refreshTraffic()
		m.err = ""
		if msg.Result.Copied {
			m.status = fmt.Sprintf("public: %s  copied to clipboard", msg.Result.Session.URL)
		} else {
			m.status = fmt.Sprintf("public: %s", msg.Result.Session.URL)
		}
		return m, tickCmd()
	case stopDoneMsg:
		if msg.Err != nil {
			m.err = msg.Err.Error()
			m.status = ""
			return m, nil
		}
		m.active = nil
		m.traffic = state.Traffic{}
		m.err = ""
		m.status = "stopped funnelr"
	case tickMsg:
		m.frame++
		m.refreshTraffic()
		return m, tickCmd()
	}
	return m, nil
}

func (m Model) startExpose(port int) (tea.Model, tea.Cmd) {
	if m.expose == nil {
		m.err = "expose is unavailable"
		return m, nil
	}
	m.exposing = true
	m.err = ""
	m.status = fmt.Sprintf("exposing localhost:%d...", port)
	return m, exposeCmd(m.expose, port)
}

func (m Model) updateCustom(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.customMode = false
		m.customValue = ""
		m.err = ""
	case "enter", "return":
		port, err := strconv.Atoi(strings.TrimSpace(m.customValue))
		if err != nil || port <= 0 || port > 65535 {
			m.err = "enter a port between 1 and 65535"
			return m, nil
		}
		m.customMode = false
		m.customValue = ""
		return m.startExpose(port)
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
		b.WriteString(activeStyle.Render(m.activeLine()))
		b.WriteString("\n")
	} else {
		b.WriteString(dimStyle.Render("no active tunnel"))
		b.WriteString("\n")
	}
	if m.status != "" {
		b.WriteString(dimStyle.Render(m.status))
		b.WriteString("\n")
	}
	if m.err != "" {
		b.WriteString(errorStyle.Render(m.err))
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

	if m.exposing {
		b.WriteString(dimStyle.Render(spinnerFrame(m.frame) + " working..."))
		b.WriteString("\n")
	} else if len(m.ports) == 0 {
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
	b.WriteString(dimStyle.Render("↑/↓ move  enter/space expose  c custom  s stop  l logs  q quit"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) Result() Result {
	return m.result
}

func (m *Model) refreshTraffic() {
	if m.active == nil || m.active.StatsPath == "" {
		m.traffic = state.Traffic{}
		return
	}
	traffic, err := state.LoadTraffic(m.active.StatsPath)
	if err == nil {
		m.traffic = traffic
		return
	}
	m.traffic = state.Traffic{}
}

func (m Model) activeLine() string {
	return fmt.Sprintf("%s localhost:%d ⇄ %s  ↓ %s  ↑ %s  %d req",
		spinnerFrame(m.frame),
		m.active.TargetPort,
		m.active.URL,
		humanBytes(m.traffic.RequestBytes),
		humanBytes(m.traffic.ResponseBytes),
		m.traffic.Requests,
	)
}

func spinnerFrame(frame int) string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	return frames[frame%len(frames)]
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PB", value/unit)
}

type exposeDoneMsg struct {
	Result funnel.ExposeResult
	Err    error
}

type stopDoneMsg struct {
	Err error
}

type tickMsg time.Time

func exposeCmd(expose ExposeFunc, port int) tea.Cmd {
	return func() tea.Msg {
		result, err := expose(context.Background(), port, nil)
		return exposeDoneMsg{Result: result, Err: err}
	}
}

func stopCmd(stop StopFunc) tea.Cmd {
	return func() tea.Msg {
		return stopDoneMsg{Err: stop(context.Background())}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
