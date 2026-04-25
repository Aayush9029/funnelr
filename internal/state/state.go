package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	ToolName = "funnelr"
	BaseDir  = "/tmp/funnelr"
)

type Session struct {
	TargetPort int       `json:"target_port"`
	ProxyPort  int       `json:"proxy_port"`
	PID        int       `json:"pid"`
	URL        string    `json:"url"`
	SessionID  string    `json:"session_id"`
	LogPath    string    `json:"log_path"`
	StatsPath  string    `json:"stats_path"`
	StartedAt  time.Time `json:"started_at"`
}

type Traffic struct {
	Requests      int64     `json:"requests"`
	RequestBytes  int64     `json:"request_bytes"`
	ResponseBytes int64     `json:"response_bytes"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func Dir() string {
	return BaseDir
}

func Path() string {
	return filepath.Join(Dir(), "state.json")
}

func NewSessionID(t time.Time) string {
	return t.UTC().Format("20060102T150405")
}

func LogPath(port int, sessionID string) string {
	return filepath.Join(Dir(), itoa(port)+"-"+sessionID+".log")
}

func StatsPath(port int, sessionID string) string {
	return filepath.Join(Dir(), itoa(port)+"-"+sessionID+".stats.json")
}

func DaemonLogPath(port int, sessionID string) string {
	return filepath.Join(Dir(), itoa(port)+"-"+sessionID+".daemon.log")
}

func LatestLogPath(port int) string {
	matches, err := filepath.Glob(filepath.Join(Dir(), itoa(port)+"-*.log"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	sort.Slice(matches, func(i, j int) bool {
		ai, _ := os.Stat(matches[i])
		aj, _ := os.Stat(matches[j])
		if ai == nil || aj == nil {
			return false
		}
		return ai.ModTime().After(aj.ModTime())
	})
	return matches[0]
}

func Save(s Session) error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), data, 0o644)
}

func Load() (Session, error) {
	data, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return Session{}, os.ErrNotExist
	}
	if err != nil {
		return Session{}, err
	}
	var s Session
	return s, json.Unmarshal(data, &s)
}

func Clear() error {
	err := os.Remove(Path())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func LoadTraffic(path string) (Traffic, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Traffic{}, err
	}
	var traffic Traffic
	return traffic, json.Unmarshal(data, &traffic)
}

func SaveTraffic(path string, traffic Traffic) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(traffic)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
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
