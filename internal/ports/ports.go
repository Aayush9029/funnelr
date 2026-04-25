package ports

import (
	"net"
	"sort"
	"time"
)

var Defaults = []int{
	80, 1000, 3000, 3001, 3002, 3003, 4000, 4173, 4321, 4444,
	5000, 5173, 5432, 5555, 6006, 7000, 8000, 8001, 8080, 8081,
	8443, 8888, 9000, 9999, 10000, 10001, 30001,
}

type Port struct {
	Number int
	Open   bool
}

func Scan(defaults []int, timeout time.Duration) []Port {
	if len(defaults) == 0 {
		defaults = Defaults
	}
	seen := map[int]bool{}
	var out []Port
	for _, p := range defaults {
		if p <= 0 || p > 65535 || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, Port{Number: p, Open: IsOpen(p, timeout)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

func OpenOnly(defaults []int, timeout time.Duration) []Port {
	scanned := Scan(defaults, timeout)
	out := scanned[:0]
	for _, p := range scanned {
		if p.Open {
			out = append(out, p)
		}
	}
	return out
}

func IsOpen(port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", itoa(port)), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[n:])
}
