package proxylog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProxyLogsMetadata(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	targetPort := mustPort(t, upstream.URL)
	proxyPort, err := FreePort()
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "3000.log")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- Server{TargetPort: targetPort, ProxyPort: proxyPort, LogPath: logPath}.Serve(ctx)
	}()
	waitPort(t, proxyPort)

	resp, err := http.Get("http://127.0.0.1:" + itoa(proxyPort) + "/hello?x=1")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("proxy did not shut down")
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	line := string(data)
	for _, want := range []string{"method=GET", `path="/hello?x=1"`, "status=201", "response_bytes=2"} {
		if !strings.Contains(line, want) {
			t.Fatalf("log %q missing %q", line, want)
		}
	}
}

func waitPort(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://127.0.0.1:" + itoa(port) + "/__probe__")
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("port %d did not open", port)
}

func mustPort(t *testing.T, rawURL string) int {
	t.Helper()
	idx := strings.LastIndex(rawURL, ":")
	if idx < 0 {
		t.Fatalf("url has no port: %s", rawURL)
	}
	var port int
	for _, r := range rawURL[idx+1:] {
		if r < '0' || r > '9' {
			break
		}
		port = port*10 + int(r-'0')
	}
	if port == 0 {
		t.Fatalf("url has invalid port: %s", rawURL)
	}
	return port
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
