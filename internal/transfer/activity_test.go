package transfer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestGateLimitAppliesToNewWork(t *testing.T) {
	g := NewGate(1)
	release, err := g.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err = g.Acquire(ctx); err == nil {
		t.Fatal("second transfer passed a full gate")
	}
	release()
	release2, err := g.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release2()
	g.SetLimit(6)
	if g.Limit() != 5 {
		t.Fatalf("limit was not clamped: %d", g.Limit())
	}
}

func TestActivityRecoveryMarksInterruptedAndIgnoresMalformed(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "transfers.json")
	if err := os.WriteFile(p, []byte(`{"version":1,"entries":[{"id":"a","status":"Running","remotePath":"/safe/file","localPath":"C:/safe/file","direction":"Download"},{"id":"b","status":"Completed","remotePath":"/done","localPath":"C:/done"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	m := OpenActivityManager(p, NewGate(2))
	items := m.List()
	if items[0].Status != "Interrupted" {
		t.Fatalf("running transfer status = %s", items[0].Status)
	}
	if len(items) != 1 || items[0].ID != "a" {
		t.Fatalf("session transfer history should not be restored: %#v", items)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(`"Completed"`)) || bytes.Contains(data, []byte(`"Running"`)) {
		t.Fatalf("recovery file contains non-recovery history: %s", data)
	}
	if err := os.WriteFile(p, []byte("partial json"), 0600); err != nil {
		t.Fatal(err)
	}
	m = OpenActivityManager(p, NewGate(2))
	if len(m.List()) != 0 {
		t.Fatal("malformed transfer history should be ignored")
	}
}

func TestSafeRemotePath(t *testing.T) {
	for _, p := range []string{"/root/../secret", "/root/sub/../../secret", "/root\\secret"} {
		if err := safeRemotePath(p); err == nil {
			t.Errorf("accepted unsafe path %q", p)
		}
	}
	if err := safeRemotePath("/root/sub/file.txt"); err != nil {
		t.Fatalf("rejected valid path: %v", err)
	}
}

type activityMock struct {
	mu      sync.Mutex
	files   map[string][]byte
	block   bool
	started chan struct{}
}

func (m *activityMock) ListDirectory(_ context.Context, dir string) ([]RemoteFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []RemoteFile{}
	for p, b := range m.files {
		if path.Dir(p) == dir {
			out = append(out, RemoteFile{Path: p, Size: int64(len(b)), SizeKnown: true})
		}
	}
	return out, nil
}
func (m *activityMock) Download(ctx context.Context, p string, w io.Writer) error {
	if m.block {
		close(m.started)
		<-ctx.Done()
		return ctx.Err()
	}
	m.mu.Lock()
	b, ok := m.files[p]
	m.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	_, e := w.Write(b)
	return e
}
func (m *activityMock) Upload(ctx context.Context, p string, r io.Reader) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	b, e := io.ReadAll(r)
	if e != nil {
		return e
	}
	m.mu.Lock()
	m.files[p] = b
	m.mu.Unlock()
	return nil
}
func waitStatus(t *testing.T, m *ActivityManager, id, want string) Activity {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		for _, a := range m.List() {
			if a.ID == id && a.Status == want {
				return a
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("transfer %s did not reach %s: %#v", id, want, m.List())
	return Activity{}
}

func TestActivityDownloadPublishesVerifiedFile(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "remote.txt")
	client := &activityMock{files: map[string][]byte{"/root/remote.txt": []byte("verified")}}
	m := OpenActivityManager(filepath.Join(root, "activity.json"), NewGate(2))
	job, e := m.AddDownload(1, "test", "/root/remote.txt", src, int64(len("verified")), "Ask", client)
	if e != nil {
		t.Fatal(e)
	}
	done := waitStatus(t, m, job.ID, "Completed")
	if done.Verification != "size" {
		t.Fatalf("verification = %q", done.Verification)
	}
	b, e := os.ReadFile(src)
	if e != nil || string(b) != "verified" {
		t.Fatalf("download result %q, %v", b, e)
	}
	if _, e = os.Stat(src + ".quickftp-" + job.ID + ".partial"); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("partial file remains: %v", e)
	}
}

func TestActivityAskConflictPausesOnlyUntilResolved(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "existing.txt")
	if e := os.WriteFile(dest, []byte("keep"), 0600); e != nil {
		t.Fatal(e)
	}
	client := &activityMock{files: map[string][]byte{"/root/existing.txt": []byte("new")}}
	m := OpenActivityManager(filepath.Join(root, "activity.json"), NewGate(2))
	job, e := m.AddDownload(1, "test", "/root/existing.txt", dest, 3, "Ask", client)
	if e != nil {
		t.Fatal(e)
	}
	waitStatus(t, m, job.ID, "Waiting")
	if e = m.ResolveConflict(job.ID, "Skip"); e != nil {
		t.Fatal(e)
	}
	waitStatus(t, m, job.ID, "Skipped")
	b, _ := os.ReadFile(dest)
	if !bytes.Equal(b, []byte("keep")) {
		t.Fatalf("existing file changed: %q", b)
	}
}

func TestActivityUploadVerifiesRemoteSize(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "upload.txt")
	if e := os.WriteFile(src, []byte("content"), 0600); e != nil {
		t.Fatal(e)
	}
	client := &activityMock{files: map[string][]byte{}}
	m := OpenActivityManager(filepath.Join(root, "activity.json"), NewGate(1))
	job, e := m.AddUpload(2, "test", "/root/upload.txt", src, "Ask", client)
	if e != nil {
		t.Fatal(e)
	}
	done := waitStatus(t, m, job.ID, "Completed")
	if done.Verification != "remote size" {
		t.Fatalf("verification = %q", done.Verification)
	}
	if !bytes.Equal(client.files["/root/upload.txt"], []byte("content")) {
		t.Fatal("upload content mismatch")
	}
}

func TestCancelledDownloadRemovesPartial(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "partial.txt")
	client := &activityMock{files: map[string][]byte{}, block: true, started: make(chan struct{})}
	m := OpenActivityManager(filepath.Join(root, "activity.json"), NewGate(1))
	job, e := m.AddDownload(1, "test", "/root/partial.txt", dest, 10, "Ask", client)
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-client.started:
	case <-time.After(time.Second):
		t.Fatal("download did not start")
	}
	if e = m.Cancel(job.ID); e != nil {
		t.Fatal(e)
	}
	waitStatus(t, m, job.ID, "Cancelled")
	if _, e = os.Stat(dest + ".quickftp-" + job.ID + ".partial"); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("cancelled partial file remains: %v", e)
	}
}
