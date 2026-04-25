package ui

import (
	"fmt"
	"os"
)

const (
	Green = "\033[32m"
	Red   = "\033[31m"
	Cyan  = "\033[36m"
	Dim   = "\033[2m"
	Bold  = "\033[1m"
	Reset = "\033[0m"
)

func IsTTY() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func Header(msg string) {
	if IsTTY() {
		fmt.Fprintf(os.Stderr, "\n%s%s%s%s\n", Cyan, Bold, msg, Reset)
	}
}

func Status(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s→ %s%s\n", Dim, fmt.Sprintf(format, args...), Reset)
}

func Success(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s✓ %s%s\n", Green, fmt.Sprintf(format, args...), Reset)
}

func Error(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s✗ %s%s\n", Red, fmt.Sprintf(format, args...), Reset)
}
