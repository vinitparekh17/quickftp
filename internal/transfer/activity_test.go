package transfer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

func TestSkipDownloadOnlySkipsExistingDestination(t *testing.T) {
	root := t.TempDir()
	m := OpenActivityManager("", NewGate(1))
	defer m.Close()
	client := &activityMock{files: map[string][]byte{"/new.txt": []byte("new")}}
	job, err := m.AddDownload(1, "test", "/new.txt", filepath.Join(root, "new.txt"), 3, "Skip", client)
	if err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, job.ID, "Completed")
}

func TestActivityCloseCancelsWorkersAndRejectsNewWork(t *testing.T) {
	m := OpenActivityManager("", NewGate(1))
	client := &activityMock{block: true, started: make(chan struct{})}
	local := filepath.Join(t.TempDir(), "download")
	job, err := m.AddDownload(1, "test", "/file", local, 10, "Ask", client)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-client.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	m.Close()
	waitStatus(t, m, job.ID, "Cancelled")
	if _, err := m.AddDownload(1, "test", "/file", local, 10, "Ask", client); err == nil {
		t.Fatal("closed manager accepted work")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.clients)+len(m.cancels)+len(m.conflicts) != 0 {
		t.Fatal("worker resources retained after close")
	}
}

func TestCancelledContextCannotAcquireAvailableGate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if release, err := NewGate(1).Acquire(ctx); err == nil {
		release()
		t.Fatal("cancelled work acquired gate")
	}
}

func TestFailureHistoryAndBatchSurviveRestart(t *testing.T) {
	file := filepath.Join(t.TempDir(), "activity.json")
	m := OpenActivityManager(file, NewGate(1))
	batch := NewTransferBatch("API publish folder")
	job, err := m.AddDownload(1, "test", "/missing.dll", filepath.Join(t.TempDir(), "missing.dll"), 1, "Ask", &activityMock{files: map[string][]byte{}}, batch)
	if err != nil {
		t.Fatal(err)
	}
	failed := waitStatus(t, m, job.ID, "Failed")
	m.Close()
	restored := OpenActivityManager(file, NewGate(1)).List()
	if len(restored) != 1 || restored[0].Status != "Failed" || restored[0].Error != failed.Error || restored[0].BatchID != batch.ID || restored[0].BatchName != batch.Name {
		t.Fatalf("failure or batch metadata lost on restart: %+v", restored)
	}
}

func TestFailureHistoryIsNotPrunedWithCompletedRows(t *testing.T) {
	m := OpenActivityManager("", NewGate(1))
	for i := 0; i < 3252; i++ {
		m.entries = append(m.entries, Activity{ID: fmt.Sprint(i), Direction: "Upload", Status: "Failed", Error: "original diagnostic"})
	}
	m.pruneHistoryLocked()
	if len(m.entries) != 3252 {
		t.Fatalf("valuable failure history pruned: %d", len(m.entries))
	}
}

type blockedUploadClient struct{ activityMock }

func (c *blockedUploadClient) Upload(ctx context.Context, _ string, _ io.Reader) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestBulkUploadValidatesBeforeAdmission(t *testing.T) {
	m := OpenActivityManager("", NewGate(1))
	defer m.Close()
	file := filepath.Join(t.TempDir(), "good.txt")
	if err := os.WriteFile(file, []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := m.AddUploads(1, "test", []UploadRequest{{RemotePath: "/good", LocalPath: file}, {RemotePath: "/missing", LocalPath: file + ".missing"}}, "Ask", &blockedUploadClient{}, NewTransferBatch("folder"))
	if err == nil || len(m.List()) != 0 {
		t.Fatalf("invalid batch admitted partial work: %v, %+v", err, m.List())
	}
}
func TestClearTransferHistoryKeepsActiveAndOtherConnections(t *testing.T) {
	m := OpenActivityManager(filepath.Join(t.TempDir(), "activity.json"), NewGate(1))
	m.entries = []Activity{{ID: "failed", Direction: "Upload", ConnectionID: 1, Status: "Failed"}, {ID: "running", Direction: "Upload", ConnectionID: 1, Status: "Running"}, {ID: "waiting", Direction: "Download", ConnectionID: 1, Status: "Waiting"}, {ID: "other", Direction: "Upload", ConnectionID: 2, Status: "Interrupted"}, {ID: "deployment", Direction: "Deployment upload", ConnectionID: 1, Status: "Failed"}}
	removed, err := m.ClearHistory(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "failed" || len(m.List()) != 4 {
		t.Fatalf("incorrect scoped clear: %v, %+v", removed, m.List())
	}
}

type sizedActivityClient struct {
	activityMock
	lists, sizes int
}

func (c *sizedActivityClient) ListDirectory(ctx context.Context, dir string) ([]RemoteFile, error) {
	c.lists++
	return c.activityMock.ListDirectory(ctx, dir)
}
func (c *sizedActivityClient) FileSize(_ context.Context, name string) (int64, error) {
	c.sizes++
	c.mu.Lock()
	defer c.mu.Unlock()
	return int64(len(c.files[name])), nil
}
func TestUploadVerifiesWithSizeWithoutSecondDirectoryListing(t *testing.T) {
	m := OpenActivityManager("", NewGate(1))
	defer m.Close()
	file := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(file, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	client := &sizedActivityClient{activityMock: activityMock{files: map[string][]byte{}}}
	job, err := m.AddUpload(1, "test", "/payload", file, "Ask", client)
	if err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, job.ID, "Completed")
	if client.lists != 1 || client.sizes != 1 {
		t.Fatalf("listing/SIZE calls: %d/%d", client.lists, client.sizes)
	}
}

func BenchmarkDirectoryQueue(b *testing.B) {
	for _, bulk := range []bool{false, true} {
		name := "individual"
		if bulk {
			name = "bulk"
		}
		b.Run(name, func(b *testing.B) {
			for iteration := 0; iteration < b.N; iteration++ {
				b.StopTimer()
				dir := b.TempDir()
				file := filepath.Join(dir, "payload")
				if err := os.WriteFile(file, []byte("test"), 0600); err != nil {
					b.Fatal(err)
				}
				m := OpenActivityManager(filepath.Join(dir, "recovery.json"), NewGate(1))
				for i := 0; i < 3252; i++ {
					m.entries = append(m.entries, Activity{ID: fmt.Sprint(i), ConnectionID: 2, Direction: "Upload", Status: "Interrupted", RemotePath: fmt.Sprintf("/site/nested/file-%d.dll", i), LocalPath: "C:/old/publish/nested/file.dll", Error: "original saved diagnostic"})
				}
				m.persistLocked()
				requests := make([]UploadRequest, 100)
				for i := range requests {
					requests[i] = UploadRequest{RemotePath: fmt.Sprintf("/file-%d", i), LocalPath: file}
				}
				client := &blockedUploadClient{}
				batch := NewTransferBatch("folder")
				b.StartTimer()
				if bulk {
					if _, err := m.AddUploads(1, "test", requests, "Ask", client, batch); err != nil {
						b.Fatal(err)
					}
				} else {
					for _, request := range requests {
						if _, err := m.AddUpload(1, "test", request.RemotePath, request.LocalPath, "Ask", client, batch); err != nil {
							b.Fatal(err)
						}
					}
				}
				b.StopTimer()
				m.Close()
			}
		})
	}
}

func TestBulkUploadDoesNotStartIfRecoveryQueueCannotBeSaved(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	m := OpenActivityManager(filepath.Join(blocker, "queue.json"), NewGate(1))
	defer m.Close()
	file := filepath.Join(dir, "payload")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := m.AddUploads(1, "test", []UploadRequest{{LocalPath: file, RemotePath: "/file"}}, "Ask", &blockedUploadClient{}, NewTransferBatch("folder"))
	if err == nil || len(m.List()) != 0 || len(m.cancels) != 0 {
		t.Fatalf("undurable batch admitted: %v, %+v", err, m.List())
	}
}
