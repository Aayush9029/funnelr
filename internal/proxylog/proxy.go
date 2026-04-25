package proxylog

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Aayush9029/funnelr/internal/state"
)

type Server struct {
	TargetPort int
	ProxyPort  int
	LogPath    string
	StatsPath  string
}

func FreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func (s Server) Serve(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.LogPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(s.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	logger := log.New(file, "", 0)
	stats := &trafficStats{path: s.StatsPath}

	target, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", s.TargetPort))
	if err != nil {
		return err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		originalDirector(r)
		r.Host = target.Host
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Printf("%s method=%s path=%q status=502 duration_ms=0 request_bytes=%d response_bytes=0 remote=%q error=%q",
			time.Now().Format(time.RFC3339), r.Method, r.URL.RequestURI(), r.ContentLength, r.RemoteAddr, err)
		stats.add(requestSize(r), 0)
		http.Error(w, err.Error(), http.StatusBadGateway)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingWriter{ResponseWriter: w, status: http.StatusOK}
		proxy.ServeHTTP(lw, r)
		logger.Printf("%s method=%s path=%q status=%d duration_ms=%d request_bytes=%d response_bytes=%d remote=%q",
			start.Format(time.RFC3339), r.Method, r.URL.RequestURI(), lw.status, time.Since(start).Milliseconds(), requestSize(r), lw.bytes, r.RemoteAddr)
		stats.add(requestSize(r), lw.bytes)
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", s.ProxyPort),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

type trafficStats struct {
	mu   sync.Mutex
	path string
	data state.Traffic
}

func (t *trafficStats) add(requestBytes, responseBytes int64) {
	if t.path == "" {
		return
	}
	if requestBytes < 0 {
		requestBytes = 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data.Requests++
	t.data.RequestBytes += requestBytes
	t.data.ResponseBytes += responseBytes
	t.data.UpdatedAt = time.Now()
	_ = state.SaveTraffic(t.path, t.data)
}

type loggingWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *loggingWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

func (w *loggingWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *loggingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func requestSize(r *http.Request) int64 {
	if r.ContentLength >= 0 {
		return r.ContentLength
	}
	if r.Body == nil {
		return 0
	}
	return -1
}
