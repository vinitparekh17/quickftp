package transfer

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"path"
	"strings"
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
}
type FTPClient struct{ Config FTPConfig }

func (c *FTPClient) dial(ctx context.Context) (*ftp.ServerConn, error) {
	port := c.Config.Port
	if port == 0 {
		port = 21
	}
	timeout := c.Config.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	opts := []ftp.DialOption{ftp.DialWithTimeout(timeout), ftp.DialWithContext(ctx)}
	if c.Config.Protocol == storage.ProtocolFTPSExplicit {
		tlsConfig := &tls.Config{ServerName: c.Config.Host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: !c.Config.TLSVerify, ClientSessionCache: tls.NewLRUClientSessionCache(8)}
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
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	opErr := fn(conn)
	quitErr := conn.Quit()
	if opErr != nil {
		return opErr
	}
	return quitErr
}
func (c *FTPClient) List(ctx context.Context, root string) ([]RemoteFile, error) {
	var files []RemoteFile
	err := c.with(ctx, func(conn *ftp.ServerConn) error { return c.walk(conn, root, 0, &files) })
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
		remote := path.Join(dir, e.Name)
		if e.Type == ftp.EntryTypeFolder {
			if err := c.walk(conn, remote, depth+1, out); err != nil {
				return err
			}
			continue
		}
		*out = append(*out, RemoteFile{Path: remote, Size: int64(e.Size), IsDir: false})
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
		return conn.Stor(name, r)
	})
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
