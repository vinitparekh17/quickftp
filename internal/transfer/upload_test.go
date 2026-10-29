package transfer

import (
	"context"
	"errors"
	"io"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recoveringFTP struct {
	*memoryFTP
	calls, failures int
	full            bool
	verificationErr error
	uploadErr       error
}

func (c *recoveringFTP) Upload(ctx context.Context, name string, r io.Reader) error {
	c.calls++
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if c.calls <= c.failures {
		if !c.full {
			data = data[:len(data)/2]
		}
		c.files[name] = data
		return c.uploadErr
	}
	c.files[name] = data
	return nil
}
func (c *recoveringFTP) Download(ctx context.Context, name string, w io.Writer) error {
	if c.verificationErr != nil {
		return c.verificationErr
	}
	return c.memoryFTP.Download(ctx, name, w)
}

func TestStagedUploadRecovery(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		failures, wantCalls  int
		full                 bool
		verifyErr, uploadErr error
		wantError            string
	}{
		{name: "partial transfer recovers", failures: 1, wantCalls: 2, uploadErr: &textproto.Error{Code: 426, Msg: "Failure reading network stream."}},
		{name: "lost reply accepted by checksum", failures: 1, wantCalls: 1, full: true, uploadErr: io.ErrUnexpectedEOF},
		{name: "unreadable state is not replayed", failures: 1, wantCalls: 1, verifyErr: errors.New("permission denied on RETR"), uploadErr: io.ErrUnexpectedEOF, wantError: "staging verification failed: permission denied on RETR"},
		{name: "permanent denial is not replayed", failures: 1, wantCalls: 1, uploadErr: &textproto.Error{Code: 550, Msg: "permission denied"}, wantError: "550"},
		{name: "bounded retry exhaustion", failures: 5, wantCalls: 3, uploadErr: &textproto.Error{Code: 426, Msg: "aborted"}, wantError: "after 3 attempt(s)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local := filepath.Join(t.TempDir(), "app.dll")
			if err := os.WriteFile(local, []byte("complete artifact"), 0600); err != nil {
				t.Fatal(err)
			}
			digest, size, err := hashFile(local)
			if err != nil {
				t.Fatal(err)
			}
			client := &recoveringFTP{memoryFTP: &memoryFTP{files: map[string][]byte{"/site/app.dll": []byte("original")}}, failures: tc.failures, full: tc.full, uploadErr: tc.uploadErr, verificationErr: tc.verifyErr}
			engine := &Engine{Client: client}
			err = engine.uploadStaged(context.Background(), 1, prepared{Artifact: Artifact{LocalPath: local}, relative: "app.dll", digest: digest, size: size}, "/site/app.dll", "/site/app.dll.quickftp-1.tmp")
			if tc.wantError == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("expected %q, got %v", tc.wantError, err)
			}
			if client.calls != tc.wantCalls {
				t.Fatalf("upload calls=%d, want %d", client.calls, tc.wantCalls)
			}
			if string(client.files["/site/app.dll"]) != "original" {
				t.Fatal("recovery modified live target")
			}
		})
	}
}
