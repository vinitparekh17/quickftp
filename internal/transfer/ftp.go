package transfer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"quickftp/internal/storage"

	"github.com/jlaffaye/ftp"
)

// FTPConfig contains the connection profile and its separately retrieved secret.
type FTPConfig struct {
	Host               string
	Port               int
	Protocol           storage.Protocol
	Username, Password string
	TLSVerify          bool
	Timeout            time.Duration
	CredentialKey      string
	ReuseConnection    bool
	ForceListHidden    bool
}

type FTPClient struct {
	Config         FTPConfig
	mu             sync.Mutex
	conn           *ftp.ServerConn
	idleTimer      *time.Timer
	idleGeneration uint64
	session        *ftpSession
	knownDirs      map[string]struct{}
}

func (c *FTPClient) dial(ctx context.Context) (*ftp.ServerConn, error) {
	port := c.Config.Port
	if port == 0 {
		port = 21
	}
	session := c.session
	control := true
	var tlsConfig *tls.Config
	opts := []ftp.DialOption{
		ftp.DialWithShutTimeout(5 * time.Second),
		ftp.DialWithDialFunc(func(network, address string) (net.Conn, error) {
			isControl := control
			control = false
			// Explicit FTPS upgrades its control socket in the library. Data sockets
			// and implicit FTPS must be wrapped here when using a custom dialer.
			var socketTLS *tls.Config
			if !isControl || port == 990 {
				socketTLS = tlsConfig
			}
			conn, err := session.dial(network, address, socketTLS)
			if err == nil && !isControl && socketTLS != nil {
				return &ftpTLSDataConn{Conn: conn.(*tls.Conn)}, nil
			}
			return conn, err
		}),
	}
	if c.Config.ForceListHidden {
		// Some servers advertise MLSD but return entries that its parser drops.
		// For the shared file browser connection, request a traditional listing
		// including hidden files so recursive deletes and walks work without
		// opening a second fallback connection, while keeping deployments unchanged.
		opts = append(opts, ftp.DialWithDisabledMLSD(true), ftp.DialWithForceListHidden(true))
	}
	if c.Config.Protocol == storage.ProtocolFTPSExplicit {
		tlsConfig = &tls.Config{ServerName: c.Config.Host, MinVersion: tls.VersionTLS12, ClientSessionCache: tls.NewLRUClientSessionCache(8), InsecureSkipVerify: !c.Config.TLSVerify}
		if port == 990 {
			opts = append(opts, ftp.DialWithTLS(tlsConfig))
		} else {
			opts = append(opts, ftp.DialWithExplicitTLS(tlsConfig))
		}
	} else if c.Config.Protocol != storage.ProtocolFTP {
		return nil, fmt.Errorf("unsupported protocol %q", c.Config.Protocol)
	}

	addr := net.JoinHostPort(c.Config.Host, strconv.Itoa(port))
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		control = true
		conn, err := ftp.Dial(addr, opts...)
		if err != nil {
			lastErr = fmt.Errorf("FTP dial: %w", err)
		} else {
			loginErr := conn.Login(c.Config.Username, c.Config.Password)
			if loginErr == nil {
				return conn, nil
			}
			_ = conn.Quit()
			lastErr = fmt.Errorf("FTP login: %w", loginErr)
		}
		session.close()
		if attempt < 2 {
			if err := sleepWithContext(ctx, time.Duration((attempt+1)*500)*time.Millisecond); err != nil {
				return nil, err
			}
		}
	}
	return nil, lastErr
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *FTPClient) with(ctx context.Context, fn func(*ftp.ServerConn) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	c.idleGeneration++
	if c.idleTimer != nil {
		c.idleTimer.Stop()
		c.idleTimer = nil
	}

	if c.session == nil {
		timeout := c.Config.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		c.session = &ftpSession{timeout: timeout, sockets: make(map[*ftpSocket]struct{})}
	}
	stop := c.session.begin(ctx)
	defer stop()

	conn := c.conn
	if conn != nil {
		if err := conn.NoOp(); err != nil {
			_ = conn.Quit()
			c.conn = nil
			conn = nil
		}
	}
	if conn == nil {
		var err error
		c.knownDirs = nil
		conn, err = c.dial(ctx)
		if err != nil {
			return err
		}
		if c.Config.ReuseConnection {
			c.conn = conn
		}
	}
	opErr := fn(conn)
	if ctx.Err() != nil {
		opErr = ctx.Err()
	}
	if c.Config.ReuseConnection && opErr == nil {
		c.closeWhenIdleLocked(30 * time.Second)
		return nil
	}
	if c.Config.ReuseConnection {
		// An operation error can mean the server completed a write but its
		// response was lost. Drop the connection, but never replay the operation.
		c.conn = nil
	}
	if conn == nil {
		return opErr
	}
	quitErr := conn.Quit()
	if opErr != nil {
		c.session.close()
		return opErr
	}
	return quitErr
}

// Test authenticates using the same transport and cancellation rules as transfers.
func (c *FTPClient) Test(ctx context.Context) error {
	return c.with(ctx, func(*ftp.ServerConn) error { return nil })
}

// Close releases a connection retained when ReuseConnection is enabled.
func (c *FTPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.idleGeneration++
	if c.idleTimer != nil {
		c.idleTimer.Stop()
		c.idleTimer = nil
	}
	if c.conn == nil {
		return nil
	}
	conn := c.conn
	c.conn = nil
	err := conn.Quit()
	c.knownDirs = nil
	return err
}

// CloseWhenIdle lets a batch of activities reuse one control connection even
// when the directory walker is still queueing later files. A new operation
// cancels this timer; an unused client is released after a short grace period.
func (c *FTPClient) CloseWhenIdle(delay time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeWhenIdleLocked(delay)
}

func (c *FTPClient) closeWhenIdleLocked(delay time.Duration) {
	c.idleGeneration++
	if c.idleTimer != nil {
		c.idleTimer.Stop()
	}
	generation := c.idleGeneration
	c.idleTimer = time.AfterFunc(delay, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if generation != c.idleGeneration {
			return
		}
		c.idleTimer = nil
		if c.conn != nil {
			_ = c.conn.Quit()
			c.conn = nil
		}
		c.knownDirs = nil
	})
}

func (c *FTPClient) List(ctx context.Context, root string) ([]RemoteFile, error) {
	var files []RemoteFile
	err := c.withRead(ctx, func(conn *ftp.ServerConn) error {
		files = files[:0]
		return c.walk(conn, root, 0, &files)
	})
	return files, err
}

// withRead retries a listing once when the reused control or data connection
// ends unexpectedly. Listings are read-only, so replaying them is safe.
func (c *FTPClient) withRead(ctx context.Context, fn func(*ftp.ServerConn) error) error {
	err := c.with(ctx, fn)
	if err == nil || ctx.Err() != nil || !isConnectionEOF(err) {
		return err
	}
	return c.with(ctx, fn)
}

func isConnectionEOF(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed)
}

func isUnsafeEntryName(name string) bool {
	return name == "" || strings.ContainsAny(name, "/\\") || strings.ContainsRune(name, 0)
}

// ListDirectory lists a single directory and preserves only metadata supplied by the server.
func (c *FTPClient) ListDirectory(ctx context.Context, dir string) ([]RemoteFile, error) {
	var files []RemoteFile
	err := c.withRead(ctx, func(conn *ftp.ServerConn) error {
		files = files[:0]
		entries, err := conn.List(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Name == "." || entry.Name == ".." {
				continue
			}
			if isUnsafeEntryName(entry.Name) {
				return fmt.Errorf("server returned unsafe directory entry")
			}
			remote := path.Join(dir, entry.Name)
			isDir := entry.Type == ftp.EntryTypeFolder
			var modified *time.Time
			if !entry.Time.IsZero() {
				t := entry.Time
				modified = &t
			}
			files = append(files, RemoteFile{Path: remote, Size: int64(entry.Size), SizeKnown: !isDir && entry.Size > 0, IsDir: isDir, Modified: modified})
		}
		return nil
	})
	return files, mapMissing(err)
}

func (c *FTPClient) walk(conn *ftp.ServerConn, dir string, depth int, out *[]RemoteFile) error {
	if depth > 64 {
		return fmt.Errorf("remote directory nesting exceeds 64 at %s", dir)
	}
	entries, err := conn.List(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		if isUnsafeEntryName(e.Name) {
			return fmt.Errorf("server returned unsafe directory entry")
		}
		remote := path.Join(dir, e.Name)
		if e.Type == ftp.EntryTypeFolder {
			if err := c.walk(conn, remote, depth+1, out); err != nil {
				return err
			}
			continue
		}
		var modified *time.Time
		if !e.Time.IsZero() {
			t := e.Time
			modified = &t
		}
		*out = append(*out, RemoteFile{Path: remote, Size: int64(e.Size), SizeKnown: e.Size > 0, IsDir: false, Modified: modified})
	}
	return nil
}

func (c *FTPClient) Download(ctx context.Context, name string, w io.Writer) error {
	return c.with(ctx, func(conn *ftp.ServerConn) error {
		r, err := conn.Retr(name)
		if err != nil {
			return mapMissing(err)
		}
		_, copyErr := io.Copy(w, r)
		closeErr := r.Close()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

// FileSize verifies a manual upload without retrieving a complete directory listing.
func (c *FTPClient) FileSize(ctx context.Context, name string) (int64, error) {
	var size int64
	err := c.withRead(ctx, func(conn *ftp.ServerConn) error { var err error; size, err = conn.FileSize(name); return err })
	return size, mapMissing(err)
}

func (c *FTPClient) Upload(ctx context.Context, name string, r io.Reader) error {
	return c.with(ctx, func(conn *ftp.ServerConn) error {
		parent := path.Dir(name)
		if parent != "." && parent != "/" {
			if err := c.ensureRemoteDirectory(conn, parent); err != nil {
				return err
			}
		}
		return conn.Stor(name, contextReader{ctx: ctx, r: r})
	})
}

// ensureRemoteDirectory remembers paths already confirmed on this client. A
// directory upload often writes many files into the same remote folders; avoid
// repeating MKD/CWD round trips for each file.
func (c *FTPClient) ensureRemoteDirectory(conn *ftp.ServerConn, dir string) error {
	if c.knownDirs == nil {
		c.knownDirs = make(map[string]struct{})
	}
	current := ""
	if strings.HasPrefix(dir, "/") {
		current = "/"
	}
	for _, part := range strings.Split(strings.Trim(dir, "/"), "/") {
		if part == "" {
			continue
		}
		current = path.Join(current, part)
		if _, ok := c.knownDirs[current]; ok {
			continue
		}
		if err := conn.MakeDir(current); err != nil {
			original, pwdErr := conn.CurrentDir()
			if pwdErr != nil {
				return pwdErr
			}
			if cdErr := conn.ChangeDir(current); cdErr != nil {
				return fmt.Errorf("create remote directory %s: %w", current, err)
			}
			if err := conn.ChangeDir(original); err != nil {
				return err
			}
		}
		c.knownDirs[current] = struct{}{}
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func (c *FTPClient) Rename(ctx context.Context, from, to string) error {
	err := c.with(ctx, func(conn *ftp.ServerConn) error { return conn.Rename(from, to) })
	if err == nil {
		c.evictKnownDir(from)
	}
	return err
}

func (c *FTPClient) evictKnownDir(target string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := strings.TrimRight(target, "/") + "/"
	for dir := range c.knownDirs {
		if dir == target || strings.HasPrefix(dir, prefix) {
			delete(c.knownDirs, dir)
		}
	}
}

func (c *FTPClient) Delete(ctx context.Context, name string) error {
	deleteOnce := func() error {
		return c.with(ctx, func(conn *ftp.ServerConn) error {
			err := conn.Delete(name)
			if err != nil {
				if errors.Is(mapMissing(err), ErrNotFound) {
					return nil
				}
				return err
			}
			return nil
		})
	}

	err := deleteOnce()
	if err == nil || ctx.Err() != nil || !isConnectionEOF(err) {
		return err
	}

	// A lost FTP reply leaves the delete outcome uncertain. Check the remote
	// file before retrying so a completed delete is not replayed blindly.
	exists, checkErr := c.remoteFileExists(ctx, name)
	if checkErr != nil {
		return fmt.Errorf("delete outcome uncertain; could not verify %s: %w", name, errors.Join(err, checkErr))
	}
	if !exists {
		return nil
	}

	err = deleteOnce()
	if err == nil || ctx.Err() != nil || !isConnectionEOF(err) {
		return err
	}
	exists, checkErr = c.remoteFileExists(ctx, name)
	if checkErr == nil && !exists {
		return nil
	}
	if checkErr != nil {
		return fmt.Errorf("delete outcome uncertain; could not verify %s: %w", name, errors.Join(err, checkErr))
	}
	return err
}

func (c *FTPClient) remoteFileExists(ctx context.Context, name string) (bool, error) {
	listing, listErr := c.ListDirectory(ctx, path.Dir(name))
	if listErr == nil {
		for _, entry := range listing {
			if entry.Path == name {
				return true, nil
			}
		}
		// Ordinary names (and hidden names when ForceListHidden is enabled) are
		// visible in a directory listing, so their absence confirms the delete
		// even on servers that do not support SIZE well.
		if c.Config.ForceListHidden || !strings.HasPrefix(path.Base(name), ".") {
			return false, nil
		}
	}

	exists := false
	err := c.withRead(ctx, func(conn *ftp.ServerConn) error {
		_, err := conn.FileSize(name)
		if err != nil {
			text := strings.ToLower(err.Error())
			if errors.Is(mapMissing(err), ErrNotFound) || strings.Contains(text, "could not get file size") {
				return nil
			}
			return err
		}
		exists = true
		return nil
	})
	return exists, err
}

// DeleteDirectory removes a remote directory recursively.
func (c *FTPClient) DeleteDirectory(ctx context.Context, name string) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = c.with(ctx, func(conn *ftp.ServerConn) error {
			// If empty, RemoveDir succeeds immediately without directory listing
			if rmdErr := conn.RemoveDir(name); rmdErr == nil {
				return nil
			}
			// Manually walk and delete all children recursively with absolute paths
			// (avoiding built-in RemoveDirRecur which calls ChangeDir and alters server CWD)
			if walkErr := c.walkDelete(ctx, conn, name, 0); walkErr == nil {
				return nil
			} else {
				return mapMissing(walkErr)
			}
		})
		if err == nil || errors.Is(err, ErrNotFound) {
			c.evictKnownDir(name)
			return nil
		}
		if ctx.Err() != nil || (!isConnectionEOF(err) && !isTransientPassiveError(err)) || attempt == 2 {
			break
		}
		if sleepErr := sleepWithContext(ctx, time.Duration((attempt+1)*250)*time.Millisecond); sleepErr != nil {
			break
		}
	}
	return err
}

func (c *FTPClient) listForDelete(ctx context.Context, conn *ftp.ServerConn, dir string) ([]*ftp.Entry, error) {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, err := conn.List(dir)
		if err == nil {
			return entries, nil
		}
		lastErr = err
		// Deep trees like node_modules open a passive data connection per folder
		// and can briefly exhaust Docker/vsftpd passive ports (connectex / refused).
		if !isTransientPassiveError(err) || attempt == 3 {
			break
		}
		if err := sleepWithContext(ctx, time.Duration((attempt+1)*150)*time.Millisecond); err != nil {
			return nil, lastErr
		}
	}
	return nil, lastErr
}

func isTransientPassiveError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "connectex") ||
		strings.Contains(s, "connection refused") ||
		strings.Contains(s, "cannot assign requested address")
}

func (c *FTPClient) walkDelete(ctx context.Context, conn *ftp.ServerConn, dir string, depth int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > 64 {
		return fmt.Errorf("remote directory nesting exceeds 64 at %s", dir)
	}
	entries, err := c.listForDelete(ctx, conn, dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		if isUnsafeEntryName(e.Name) {
			return fmt.Errorf("server returned unsafe directory entry")
		}
		child := path.Join(dir, e.Name)
		if e.Type == ftp.EntryTypeFolder {
			if err := c.walkDelete(ctx, conn, child, depth+1); err != nil {
				return err
			}
		} else {
			if err := conn.Delete(child); err != nil && !errors.Is(mapMissing(err), ErrNotFound) {
				if rmdErr := conn.RemoveDir(child); rmdErr != nil && !errors.Is(mapMissing(rmdErr), ErrNotFound) {
					return err
				}
			}
		}
	}
	if err := conn.RemoveDir(dir); err != nil {
		if errors.Is(mapMissing(err), ErrNotFound) {
			return nil
		}
		// Some servers return only a generic 550 for a non-empty directory.
		// Include the remaining entries so the UI log identifies what prevented
		// removal (for example, a hidden file omitted by the first listing).
		remaining, listErr := c.listForDelete(ctx, conn, dir)
		if listErr != nil {
			return fmt.Errorf("remove remote directory %s: %w (could not inspect remaining entries: %v)", dir, err, listErr)
		}
		var names []string
		for _, entry := range remaining {
			if entry.Name != "." && entry.Name != ".." {
				names = append(names, entry.Name)
			}
		}
		if len(names) > 0 {
			return fmt.Errorf("remove remote directory %s: %w (remaining entries: %s)", dir, err, strings.Join(names, ", "))
		}
		return fmt.Errorf("remove remote directory %s: %w (server still reports the directory as non-empty, but its listing is empty)", dir, err)
	}
	return nil
}

func mapMissing(err error) error {
	if err == nil {
		return nil
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "550") && (strings.Contains(s, "not found") || strings.Contains(s, "no such") || strings.Contains(s, "does not exist")) {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return err
}
