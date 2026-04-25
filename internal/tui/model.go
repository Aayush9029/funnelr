package tui

import (
	"context"
	"fmt"
	"os"
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
	height      int
	frame       int
	customMode  bool
	customValue string
	err         string
	status      string
	exposing    bool
	pickMode    bool
	logMode     bool
	logPort     int
	logPath     string
	logLines    []string
	result      Result
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	activeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	portStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	urlStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Underline(true)
	downStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	upStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	requestStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	spinnerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	selectorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	logTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	logLineStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
)

func NewModel(open []ports.Port, active *state.Session, expose ExposeFunc, stop StopFunc) Model {
	if len(open) == 0 {
		open = []ports.Port{}
	}
	model := Model{ports: open, active: active, expose: expose, stop: stop}
	model.refreshTraffic()
	if active != nil {
		model.openLogs()
	}
	return model
}

func (m Model) Init() tea.Cmd { return tickCmd() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			if m.logMode {
				m.logMode = false
				return m, nil
			}
			if m.pickMode && m.active != nil {
				m.pickMode = false
				return m, nil
			}
			return m, tea.Quit
		case "up", "k":
			if m.showPicker() && m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.showPicker() && m.cursor < len(m.ports)-1 {
				m.cursor++
			}
		case " ", "space", "enter", "return":
			if m.showPicker() && len(m.ports) > 0 {
				return m.activatePort(m.ports[m.cursor].Number)
			}
		case "p", "tab":
			if m.active != nil {
				m.pickMode = !m.pickMode
				if m.pickMode {
					m.logMode = false
				}
				m.err = ""
			}
		case "c":
			m.customMode = true
			m.pickMode = false
			m.logMode = false
			m.customValue = ""
			m.err = ""
		case "s":
			if m.active == nil {
				return m, nil
			}
			if m.stop == nil {
				m.err = "stop is unavailable"
				return m, nil
			}
			m.status = "stopping funnelr..."
			return m, stopCmd(m.stop)
		case "l":
			if m.active == nil {
				return m, nil
			}
			if m.logMode {
				m.logMode = false
			} else {
				m.openLogs()
			}
		}
	case exposeDoneMsg:
		m.exposing = false
		if msg.Err != nil {
			m.err = msg.Err.Error()
			m.status = ""
			return m, nil
		}
		m.active = &msg.Result.Session
		m.pickMode = false
		m.refreshTraffic()
		m.err = ""
		m.openLogs()
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
		m.pickMode = false
		m.logMode = false
		m.logLines = nil
		m.err = ""
		m.status = "stopped funnelr"
	case tickMsg:
		m.frame++
		m.refreshTraffic()
		if m.logMode {
			m.refreshLogs()
		}
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

func (m Model) activatePort(port int) (tea.Model, tea.Cmd) {
	if m.active != nil && m.active.TargetPort == port {
		m.err = ""
		m.status = fmt.Sprintf("active: %s", m.active.URL)
		m.pickMode = false
		m.openLogs()
		return m, nil
	}
	return m.startExpose(port)
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
		return m.activatePort(port)
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
		b.WriteString(m.activeLine())
		b.WriteString("\n")
	} else {
		b.WriteString(dimStyle.Render("no active tunnel"))
		b.WriteString("\n")
	}
	if m.status != "" {
		b.WriteString(statusLine(m.status))
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
	} else if m.showPicker() {
		b.WriteString(m.pickerView())
	} else if m.active != nil {
		if m.logMode {
			b.WriteString(m.logView())
		} else {
			b.WriteString(dimStyle.Render("logs hidden"))
			b.WriteString("\n")
		}
	} else if len(m.ports) == 0 {
		b.WriteString(dimStyle.Render("no common local web ports are open"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("press c to enter a custom port"))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.helpLine())
	b.WriteString("\n")
	return b.String()
}

func (m Model) Result() Result {
	return m.result
}

func (m Model) showPicker() bool {
	return m.active == nil || m.pickMode
}

func (m Model) pickerView() string {
	var b strings.Builder
	if len(m.ports) == 0 {
		b.WriteString(dimStyle.Render("no common local web ports are open"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("press c to enter a custom port"))
		b.WriteString("\n")
		return b.String()
	}
	for i, p := range m.ports {
		cursor := " "
		if i == m.cursor {
			cursor = m.cursorGlyph(p.Number)
		}
		circle := "○"
		portText := fmt.Sprintf("localhost:%d", p.Number)
		if m.active != nil && m.active.TargetPort == p.Number {
			circle = activeStyle.Render("●")
			portText = activeStyle.Render(portText)
		}
		b.WriteString(fmt.Sprintf("%s %s %s", cursor, circle, portText))
		b.WriteString("\n")
	}
	return b.String()
}

func (m *Model) openLogs() {
	port := 0
	if m.active != nil {
		port = m.active.TargetPort
	} else if len(m.ports) > 0 {
		port = m.ports[m.cursor].Number
	}
	if port <= 0 {
		m.err = "no active tunnel logs"
		return
	}
	m.logMode = true
	m.logPort = port
	m.logPath = state.LogPath(port)
	m.status = ""
	m.err = ""
	m.refreshLogs()
}

func (m *Model) refreshLogs() {
	if m.logPath == "" {
		m.logLines = nil
		return
	}
	data, err := os.ReadFile(m.logPath)
	if err != nil {
		m.logLines = []string{dimStyle.Render("no log entries yet")}
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		m.logLines = []string{dimStyle.Render("no log entries yet")}
		return
	}
	maxLines := m.logHeight()
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	m.logLines = lines
}

func (m Model) logHeight() int {
	if m.heightAvailable() < 6 {
		return 6
	}
	return m.heightAvailable()
}

func (m Model) heightAvailable() int {
	if m.height <= 0 {
		return 10
	}
	available := m.height - 8
	if available < 6 {
		return 6
	}
	return available
}

func (m Model) logView() string {
	var b strings.Builder
	b.WriteString(logTitleStyle.Render(fmt.Sprintf("logs localhost:%d", m.logPort)))
	b.WriteString(dimStyle.Render("  l/esc close  q quit"))
	b.WriteString("\n")
	for _, line := range m.logLines {
		b.WriteString(logLineStyle.Render(m.truncateLine(line)))
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) truncateLine(line string) string {
	if m.width <= 0 || len(line) <= m.width {
		return line
	}
	limit := m.width - 1
	if limit < 20 {
		limit = 20
	}
	if len(line) <= limit {
		return line
	}
	return line[:limit-1] + "…"
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
	return strings.Join([]string{
		activeStyle.Render("●"),
		portStyle.Render(fmt.Sprintf("localhost:%d", m.active.TargetPort)),
		dimStyle.Render("⇄"),
		urlStyle.Render(m.active.URL),
		downStyle.Render("↓ " + humanBytes(m.traffic.RequestBytes)),
		upStyle.Render("↑ " + humanBytes(m.traffic.ResponseBytes)),
		requestStyle.Render(fmt.Sprintf("%d req", m.traffic.Requests)),
	}, "  ")
}

func statusLine(text string) string {
	if strings.HasPrefix(text, "active: ") {
		url := strings.TrimPrefix(text, "active: ")
		return dimStyle.Render("active: ") + urlStyle.Render(url)
	}
	if strings.HasPrefix(text, "public: ") {
		url := strings.TrimPrefix(text, "public: ")
		suffix := ""
		if before, after, ok := strings.Cut(url, "  "); ok {
			url = before
			suffix = "  " + dimStyle.Render(after)
		}
		return dimStyle.Render("public: ") + urlStyle.Render(url) + suffix
	}
	return dimStyle.Render(text)
}

func (m Model) cursorGlyph(port int) string {
	if m.active != nil && m.active.TargetPort != port {
		return requestStyle.Render("⇄")
	}
	return selectorStyle.Render("›")
}

func (m Model) helpLine() string {
	if m.showPicker() {
		action := "enter/space expose"
		selectedActive := m.active != nil && len(m.ports) > 0 && m.cursor < len(m.ports) && m.active.TargetPort == m.ports[m.cursor].Number
		if selectedActive {
			action = "selected active"
		} else if m.active != nil {
			action = "enter/space swap"
		}
		parts := []string{
			dimStyle.Render("↑/↓ move"),
			selectorStyle.Render(action),
			dimStyle.Render("c custom"),
			dimStyle.Render("q quit"),
		}
		if m.active != nil {
			parts = append(parts[:3], append([]string{
				dimStyle.Render("esc close picker"),
				dimStyle.Render("s stop"),
				dimStyle.Render("l logs"),
			}, parts[3:]...)...)
		}
		return strings.Join(parts, dimStyle.Render("  "))
	}
	logAction := "l show logs"
	if m.logMode {
		logAction = "l/esc hide logs"
	}
	parts := []string{
		selectorStyle.Render("p pick/swap"),
		dimStyle.Render("c custom"),
		dimStyle.Render(logAction),
		dimStyle.Render("s stop"),
		dimStyle.Render("q quit"),
	}
	return strings.Join(parts, dimStyle.Render("  "))
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
