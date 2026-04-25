package clipboard

import (
	"io"
	"os"
	"os/exec"
	"runtime"
)

func Copy(text string) bool {
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("pbcopy")
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return false
		}
		if err := cmd.Start(); err != nil {
			return false
		}
		_, _ = io.WriteString(stdin, text)
		_ = stdin.Close()
		return cmd.Wait() == nil
	}

	if os.Getenv("TMUX") != "" || os.Getenv("TERM_PROGRAM") != "" {
		_, err := io.WriteString(os.Stdout, "\033]52;c;"+base64Text(text)+"\a")
		return err == nil
	}
	return false
}

func base64Text(s string) string {
	const table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	in := []byte(s)
	out := make([]byte, 0, (len(in)+2)/3*4)
	for i := 0; i < len(in); i += 3 {
		var b [3]byte
		n := copy(b[:], in[i:])
		v := uint(b[0])<<16 | uint(b[1])<<8 | uint(b[2])
		out = append(out, table[(v>>18)&63], table[(v>>12)&63])
		if n > 1 {
			out = append(out, table[(v>>6)&63])
		} else {
			out = append(out, '=')
		}
		if n > 2 {
			out = append(out, table[v&63])
		} else {
			out = append(out, '=')
		}
	}
	return string(out)
}
