package transfer

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"sync"
	"time"
)

// ftpSession owns the sockets behind one control connection, including its data
// channels. Operations are serialized by FTPClient.mu; cancellation only touches
// the sockets, never the FTP library's unsynchronized protocol state.
type ftpSession struct {
	mu      sync.Mutex
	ctx     context.Context
	sockets map[*ftpSocket]struct{}
	timeout time.Duration
}

type ftpSocket struct {
	net.Conn
	session                     *ftpSession
	deadlineMu                  sync.Mutex
	readDeadline, writeDeadline time.Time
}

func (s *ftpSession) begin(ctx context.Context) func() {
	s.mu.Lock()
	s.ctx = ctx
	for socket := range s.sockets {
		_ = socket.SetDeadline(time.Time{})
	}
	s.mu.Unlock()
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { s.close(); close(done) })
	return func() {
		if !stop() {
			<-done
		}
	}
}

func (s *ftpSession) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for socket := range s.sockets {
		_ = socket.Conn.Close()
		delete(s.sockets, socket)
	}
}

func (s *ftpSession) dial(network, address string, tlsConfig *tls.Config) (net.Conn, error) {
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	conn, err := (&net.Dialer{Timeout: s.timeout}).DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	socket := &ftpSocket{Conn: conn, session: s}
	s.mu.Lock()
	if err = ctx.Err(); err == nil {
		s.sockets[socket] = struct{}{}
	}
	s.mu.Unlock()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if tlsConfig != nil {
		return tls.Client(socket, tlsConfig), nil
	}
	return socket, nil
}

func (c *ftpSocket) Read(p []byte) (int, error) {
	c.deadlineMu.Lock()
	deadline := earlierDeadline(time.Now().Add(c.session.timeout), c.readDeadline)
	_ = c.Conn.SetReadDeadline(deadline)
	c.deadlineMu.Unlock()
	return c.Conn.Read(p)
}
func (c *ftpSocket) Write(p []byte) (int, error) {
	c.deadlineMu.Lock()
	deadline := earlierDeadline(time.Now().Add(c.session.timeout), c.writeDeadline)
	_ = c.Conn.SetWriteDeadline(deadline)
	c.deadlineMu.Unlock()
	return c.Conn.Write(p)
}
func (c *ftpSocket) Close() error {
	c.session.mu.Lock()
	delete(c.session.sockets, c)
	c.session.mu.Unlock()
	return c.Conn.Close()
}

func earlierDeadline(idle, explicit time.Time) time.Time {
	if !explicit.IsZero() && explicit.Before(idle) {
		return explicit
	}
	return idle
}
func (c *ftpSocket) SetDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	c.readDeadline, c.writeDeadline = t, t
	return c.Conn.SetDeadline(t)
}
func (c *ftpSocket) SetReadDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	c.readDeadline = t
	return c.Conn.SetReadDeadline(t)
}
func (c *ftpSocket) SetWriteDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	c.writeDeadline = t
	return c.Conn.SetWriteDeadline(t)
}

// Complete the TLS close-notify exchange before closing TCP. Closing a Windows
// socket with unread peer TLS records can reset the connection and truncate an
// otherwise successful upload on strict FTPS servers.
type ftpTLSDataConn struct{ *tls.Conn }

func (c *ftpTLSDataConn) Close() error {
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	notifyErr := c.CloseWrite()
	if notifyErr == nil {
		_, _ = io.Copy(io.Discard, c.Conn)
	}
	closeErr := c.Conn.Close()
	if notifyErr != nil {
		return notifyErr
	}
	return closeErr
}
