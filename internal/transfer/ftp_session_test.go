package transfer

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"quickftp/internal/storage"
)

func TestFTPSessionCancellationClosesAllSockets(t *testing.T) {
	s := &ftpSession{timeout: time.Second, sockets: make(map[*ftpSocket]struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	stop := s.begin(ctx)
	defer stop()
	for range 2 {
		local, remote := net.Pipe()
		defer remote.Close()
		socket := &ftpSocket{Conn: local, session: s}
		s.sockets[socket] = struct{}{}
	}
	cancel()
	stop()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sockets) != 0 {
		t.Fatal("cancelled session retained sockets")
	}
}

func TestFTPSocketTimeout(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	s := &ftpSession{timeout: 20 * time.Millisecond, sockets: make(map[*ftpSocket]struct{})}
	socket := &ftpSocket{Conn: local, session: s}
	defer socket.Close()
	_, err := socket.Read(make([]byte, 1))
	var timeout net.Error
	if !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatalf("expected idle timeout, got %v", err)
	}
}

func TestFTPCancelStalledGreeting(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	client := &FTPClient{Config: FTPConfig{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, Protocol: storage.ProtocolFTP}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- client.Test(ctx) }()
	select {
	case conn := <-accepted:
		defer conn.Close()
	case <-time.After(time.Second):
		t.Fatal("connection not accepted")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not interrupt FTP greeting")
	}
}

func TestFTPSessionSocketCloseUnregisters(t *testing.T) {
	s := &ftpSession{timeout: time.Second, sockets: make(map[*ftpSocket]struct{})}
	local, remote := net.Pipe()
	defer remote.Close()
	socket := &ftpSocket{Conn: local, session: s}
	s.sockets[socket] = struct{}{}
	if err := socket.Close(); err != nil {
		t.Fatal(err)
	}
	if len(s.sockets) != 0 {
		t.Fatal("closed data socket retained")
	}
	if _, err := remote.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("peer was not closed: %v", err)
	}
}

// Exercise real control and passive data sockets without an external FTP host.
func TestFTPCancelStalledDataTransfer(t *testing.T) {
	for _, upload := range []bool{false, true} {
		name := "download"
		if upload {
			name = "upload"
		}
		t.Run(name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			ready := make(chan struct{})
			serverDone := make(chan struct{})
			stopServer := make(chan struct{})
			defer close(stopServer)
			go func() {
				defer close(serverDone)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				fmt.Fprint(conn, "220 ready\r\n")
				scanner := bufio.NewScanner(conn)
				var data net.Listener
				defer func() {
					if data != nil {
						data.Close()
					}
				}()
				for scanner.Scan() {
					command := strings.Fields(scanner.Text())[0]
					switch command {
					case "USER":
						fmt.Fprint(conn, "230 logged in\r\n")
					case "FEAT":
						fmt.Fprint(conn, "211 no features\r\n")
					case "EPSV":
						data, err = net.Listen("tcp", "127.0.0.1:0")
						if err != nil {
							return
						}
						fmt.Fprintf(conn, "229 Entering Extended Passive Mode (|||%d|)\r\n", data.Addr().(*net.TCPAddr).Port)
					case "RETR", "STOR":
						fmt.Fprint(conn, "150 opening data\r\n")
						socket, err := data.Accept()
						if err != nil {
							return
						}
						defer socket.Close()
						close(ready)
						<-stopServer
						return
					case "QUIT":
						return
					default:
						fmt.Fprint(conn, "200 ok\r\n")
					}
				}
			}()
			client := &FTPClient{Config: FTPConfig{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, Protocol: storage.ProtocolFTP, ReuseConnection: true}}
			defer client.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				if upload {
					done <- client.Upload(ctx, "/file", endlessReader{})
				} else {
					done <- client.Download(ctx, "/file", io.Discard)
				}
			}()
			select {
			case <-ready:
			case err := <-done:
				t.Fatalf("transfer failed before data connection: %v", err)
			case <-time.After(2 * time.Second):
				t.Fatal("data connection did not start")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("expected cancellation, got %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("cancel did not interrupt transfer")
			}
			if client.conn != nil {
				t.Fatal("cancelled control connection retained")
			}
		})
	}
}

type endlessReader struct{}

func (endlessReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestFTPSocketHonorsExplicitDeadline(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	s := &ftpSession{timeout: time.Second, sockets: make(map[*ftpSocket]struct{})}
	socket := &ftpSocket{Conn: local, session: s}
	defer socket.Close()
	if err := socket.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err := socket.Read(make([]byte, 1))
	if err == nil || time.Since(start) >= 500*time.Millisecond {
		t.Fatalf("explicit deadline was ignored: %v", err)
	}
}
