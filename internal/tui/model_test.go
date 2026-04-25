package tui

import (
	"strings"
	"testing"

	"github.com/Aayush9029/funnelr/internal/ports"
	"github.com/Aayush9029/funnelr/internal/state"
)

func TestHelpLineNoActiveHidesStopAndLogs(t *testing.T) {
	model := NewModel([]ports.Port{{Number: 3000, Open: true}}, nil, nil, nil)
	help := stripANSI(model.helpLine())
	for _, disallowed := range []string{"s stop", "l logs", "swap"} {
		if strings.Contains(help, disallowed) {
			t.Fatalf("help %q contains %q", help, disallowed)
		}
	}
	if !strings.Contains(help, "enter/space expose") {
		t.Fatalf("help %q missing expose action", help)
	}
}

func TestHelpLineActiveDashboard(t *testing.T) {
	active := &state.Session{TargetPort: 3000, URL: "https://example.ts.net"}
	model := NewModel([]ports.Port{{Number: 3000, Open: true}}, active, nil, nil)
	help := stripANSI(model.helpLine())
	for _, want := range []string{"p change port", "l/esc hide logs", "s stop"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help %q missing %q", help, want)
		}
	}
}

func TestHelpLineActivePickerSelectedActive(t *testing.T) {
	active := &state.Session{TargetPort: 3000, URL: "https://example.ts.net"}
	model := NewModel([]ports.Port{{Number: 3000, Open: true}, {Number: 8000, Open: true}}, active, nil, nil)
	model.pickMode = true
	help := stripANSI(model.helpLine())
	if strings.Contains(help, "enter/space expose") || strings.Contains(help, "enter/space swap") {
		t.Fatalf("help %q should not advertise expose/swap for selected active port", help)
	}
	if !strings.Contains(help, "selected active") {
		t.Fatalf("help %q missing selected active state", help)
	}
	for _, disallowed := range []string{"s stop", "l logs"} {
		if strings.Contains(help, disallowed) {
			t.Fatalf("picker help %q should not contain dashboard action %q", help, disallowed)
		}
	}
}

func TestLogViewDoesNotDuplicateControls(t *testing.T) {
	active := &state.Session{TargetPort: 3000, URL: "https://example.ts.net"}
	model := NewModel([]ports.Port{{Number: 3000, Open: true}}, active, nil, nil)
	view := stripANSI(model.View())
	if strings.Count(view, "l/esc") != 1 {
		t.Fatalf("view %q should contain one l/esc control", view)
	}
	if strings.Contains(view, "logs localhost:3000  l/esc") {
		t.Fatalf("log header should not contain controls: %q", view)
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inEscape {
			if c >= '@' && c <= '~' {
				inEscape = false
			}
			continue
		}
		if c == 0x1b {
			inEscape = true
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
