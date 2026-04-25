package ports

import (
	"net"
	"testing"
	"time"
)

func TestIsOpen(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	if !IsOpen(port, time.Second) {
		t.Fatalf("expected %d to be open", port)
	}
}

func TestOpenOnly(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	openPort := ln.Addr().(*net.TCPAddr).Port
	closedPort := freeClosedPort(t)
	got := OpenOnly([]int{closedPort, openPort}, 100*time.Millisecond)
	if len(got) != 1 || got[0].Number != openPort {
		t.Fatalf("got %#v, want only %d", got, openPort)
	}
}

func freeClosedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}
