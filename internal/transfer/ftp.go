package transfer

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/jlaffaye/ftp"
	"quickftp/internal/storage"
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
}
type FTPClient struct {
	Config FTPConfig
	mu     sync.Mutex
	conn   *ftp.ServerConn
}

func (c *FTPClient) dial(ctx context.Context) (*ftp.ServerConn, error) {
	port := c.Config.Port
	if port == 0 {
		port = 21
	}
	timeout := c.Config.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	opts := []ftp.DialOption{ftp.DialWithTimeout(timeout), ftp.DialWithContext(ctx), ftp.DialWithShutTimeout(5 * time.Second)}
	if c.Config.Protocol == storage.ProtocolFTPSExplicit {
		tlsConfig := &tls.Config{ServerName: c.Config.Host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: !c.Config.TLSVerify}
		if port == 990 {
			opts = append(opts, ftp.DialWithTLS(tlsConfig))
		} else {
			opts = append(opts, ftp.DialWithExplicitTLS(tlsConfig))
		}
	} else if c.Config.Protocol != storage.ProtocolFTP {
		return nil, fmt.Errorf("unsupported protocol %q", c.Config.Protocol)
	}
	conn, err := ftp.Dial(fmt.Sprintf("%s:%d", c.Config.Host, port), opts...)
	if err != nil {
		return nil, err
	}
	if err = conn.Login(c.Config.Username, c.Config.Password); err != nil {
		_ = conn.Quit()
		return nil, fmt.Errorf("FTP login: %w", err)
	}
	return conn, nil
}
func (c *FTPClient) with(ctx context.Context, fn func(*ftp.ServerConn) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn := c.conn
	if conn == nil {
		var err error
		conn, err = c.dial(ctx)
		if err != nil {
			return err
		}
		if c.Config.ReuseConnection {
			c.conn = conn
		}
	}
	opErr := fn(conn)
	if c.Config.ReuseConnection && opErr == nil {
		return nil
	}
	if c.Config.ReuseConnection {
		c.conn = nil
	}
	quitErr := conn.Quit()
	if opErr != nil {
		return opErr
	}
	return quitErr
}

// Close releases a connection retained when ReuseConnection is enabled.
func (c *FTPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	conn := c.conn
	c.conn = nil
	return conn.Quit()
}
func (c *FTPClient) List(ctx context.Context, root string) ([]RemoteFile, error) {
	var files []RemoteFile
	err := c.with(ctx, func(conn *ftp.ServerConn) error { return c.walk(conn, root, 0, &files) })
	return files, err
}

// ListDirectory lists a single directory and preserves only metadata supplied by the server.
func (c *FTPClient) ListDirectory(ctx context.Context, dir string) ([]RemoteFile, error) {
	var files []RemoteFile
	err := c.with(ctx, func(conn *ftp.ServerConn) error {
		entries, err := conn.List(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Name == "." || entry.Name == ".." {
				continue
			}
			if strings.ContainsAny(entry.Name, "/\\") || strings.ContainsRune(entry.Name, 0) {
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
	return files, err
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
		if strings.ContainsAny(e.Name, "/\\") || strings.ContainsRune(e.Name, 0) {
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
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			select {
			case <-ctx.Done():
				_ = r.Close()
			case <-stop:
			}
		}()
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
func (c *FTPClient) Upload(ctx context.Context, name string, r io.Reader) error {
	return c.with(ctx, func(conn *ftp.ServerConn) error {
		parent := path.Dir(name)
		if parent != "." && parent != "/" {
			current := ""
			if strings.HasPrefix(parent, "/") {
				current = "/"
			}
			for _, part := range strings.Split(strings.Trim(parent, "/"), "/") {
				if part == "" {
					continue
				}
				current = path.Join(current, part)
				if err := conn.MakeDir(current); err != nil {
					if cdErr := conn.ChangeDir(current); cdErr != nil {
						return fmt.Errorf("create remote directory %s: %w", current, err)
					}
				}
			}
		}
		return conn.Stor(name, contextReader{ctx: ctx, r: r})
	})
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
	return c.with(ctx, func(conn *ftp.ServerConn) error { return conn.Rename(from, to) })
}
func (c *FTPClient) Delete(ctx context.Context, name string) error {
	return c.with(ctx, func(conn *ftp.ServerConn) error {
		err := conn.Delete(name)
		if err != nil {
			return mapMissing(err)
		}
		return nil
	})
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
