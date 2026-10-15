package transfer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"testing"

	"quickftp/internal/storage"
)

type memoryFTP struct {
	files                map[string][]byte
	events               []string
	failUpload           bool
	failUploadAfterWrite bool
}

func (m *memoryFTP) List(_ context.Context, root string) ([]RemoteFile, error) {
	var out []RemoteFile
	for p, b := range m.files {
		if p != root && len(p) > len(root) && p[:len(root)+1] == root+"/" {
			out = append(out, RemoteFile{Path: p, Size: int64(len(b))})
		}
	}
	return out, nil
}
func (m *memoryFTP) Download(_ context.Context, p string, w io.Writer) error {
	b, ok := m.files[p]
	if !ok {
		return ErrNotFound
	}
	_, err := w.Write(b)
	return err
}
func (m *memoryFTP) Upload(_ context.Context, p string, r io.Reader) error {
	m.events = append(m.events, "upload:"+p)
	if m.failUpload && !m.failUploadAfterWrite {
		return io.ErrUnexpectedEOF
	}
	b, err := io.ReadAll(r)
	if err == nil {
		m.files[p] = b
	}
	if m.failUpload {
		return io.ErrUnexpectedEOF
	}
	return err
}
func (m *memoryFTP) Rename(_ context.Context, a, b string) error {
	m.events = append(m.events, "rename:"+a)
	v, ok := m.files[a]
	if !ok {
		return ErrNotFound
	}
	m.files[b] = v
	delete(m.files, a)
	return nil
}
func (m *memoryFTP) Delete(_ context.Context, p string) error {
	m.events = append(m.events, "delete:"+p)
	if _, ok := m.files[p]; !ok {
		return ErrNotFound
	}
	delete(m.files, p)
	return nil
}

func testEngine(t *testing.T, remote map[string][]byte) (*Engine, *memoryFTP, int64, int64) {
	t.Helper()
	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "journal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Conn.Exec(`INSERT INTO projects(name,project_type,local_path,build_cmd,output_rel_path) VALUES('test','angular','.','','dist')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Conn.Exec(`INSERT INTO connections(name,host,port,protocol,username,remote_root,cred_target_key) VALUES('test','localhost',21,'ftp','u','/site','test')`)
	if err != nil {
		t.Fatal(err)
	}
	ftp := &memoryFTP{files: remote}
	return &Engine{DB: db, Client: ftp, RemoteRoot: "/site", BackupRoot: filepath.Join(t.TempDir(), "backups")}, ftp, 1, 1
}

func TestDeployBacksUpBeforeReplacingAndRemovesStaleFiles(t *testing.T) {
	engine, ftp, project, connection := testEngine(t, map[string][]byte{"/site/index.html": []byte("old"), "/site/obsolete.js": []byte("stale")})
	local := filepath.Join(t.TempDir(), "index.html")
	if err := os.WriteFile(local, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := engine.Deploy(context.Background(), project, connection, []Artifact{{LocalPath: local, RelativePath: "index.html"}})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected deployment id")
	}
	if string(ftp.files["/site/index.html"]) != "new" {
		t.Fatal("new artifact was not deployed")
	}
	if _, ok := ftp.files["/site/obsolete.js"]; ok {
		t.Fatal("stale file was not deleted")
	}
	backup, err := os.ReadFile(filepath.Join(engine.BackupRoot, fmt.Sprintf("deployment-%d", id), "index.html"))
	if err != nil || string(backup) != "old" {
		t.Fatalf("backup=%q err=%v", backup, err)
	}
	if len(ftp.events) < 4 || !bytes.Contains([]byte(ftp.events[0]), []byte("upload:")) || !bytes.Contains([]byte(ftp.events[len(ftp.events)-1]), []byte("delete:/site/obsolete.js")) {
		t.Fatalf("unexpected operation order: %v", ftp.events)
	}
}

func TestDeployDoesNotDeleteRemoteFilesWhenUploadFails(t *testing.T) {
	engine, ftp, p, c := testEngine(t, map[string][]byte{"/site/old.txt": []byte("old")})
	ftp.failUpload = true
	local := filepath.Join(t.TempDir(), "new.txt")
	_ = os.WriteFile(local, []byte("new"), 0600)
	_, err := engine.Deploy(context.Background(), p, c, []Artifact{{LocalPath: local, RelativePath: "new.txt"}})
	if err == nil {
		t.Fatal("expected upload failure")
	}
	if string(ftp.files["/site/old.txt"]) != "old" {
		t.Fatal("remote file changed before upload succeeded")
	}
}

func TestDeployReconcilesUploadWhoseReplyWasLost(t *testing.T) {
	engine, ftp, p, c := testEngine(t, map[string][]byte{})
	ftp.failUpload, ftp.failUploadAfterWrite = true, true
	local := filepath.Join(t.TempDir(), "new.txt")
	_ = os.WriteFile(local, []byte("new"), 0600)
	if _, err := engine.Deploy(context.Background(), p, c, []Artifact{{LocalPath: local, RelativePath: "new.txt"}}); err != nil {
		t.Fatalf("successful upload should be reconciled: %v", err)
	}
	if string(ftp.files["/site/new.txt"]) != "new" {
		t.Fatal("artifact was not promoted")
	}
}

func TestIISOfflineMarkerIsRemovedAfterDeployment(t *testing.T) {
	engine, ftp, p, c := testEngine(t, map[string][]byte{})
	engine.IISOfflinePage = "maintenance"
	local := filepath.Join(t.TempDir(), "index.html")
	_ = os.WriteFile(local, []byte("ok"), 0600)
	if _, err := engine.Deploy(context.Background(), p, c, []Artifact{{LocalPath: local, RelativePath: "index.html"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := ftp.files["/site/app_offline.htm"]; ok {
		t.Fatal("IIS offline marker remained after deployment")
	}
}

func TestRollbackRestoresBackupsAndRemovesNewArtifacts(t *testing.T) {
	engine, ftp, _, _ := testEngine(t, map[string][]byte{"/site/index.html": []byte("deployed"), "/site/new.js": []byte("new")})
	id, err := engine.DB.StartDeployment(1, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(engine.BackupRoot, fmt.Sprintf("deployment-%d", id))
	if err = os.MkdirAll(backupDir, 0700); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(backupDir, "index.html")
	if err = os.WriteFile(backup, []byte("previous"), 0600); err != nil {
		t.Fatal(err)
	}
	digest, size, err := hashFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if err = engine.DB.SetDeploymentBackupDir(id, backupDir); err != nil {
		t.Fatal(err)
	}
	if err = engine.DB.JournalFile(id, "index.html", "backup", "verified", size, digest, ""); err != nil {
		t.Fatal(err)
	}
	if err = engine.DB.JournalFile(id, "index.html", "upload", "verified", 8, "", ""); err != nil {
		t.Fatal(err)
	}
	if err = engine.DB.JournalFile(id, "new.js", "upload", "failed", 3, "", "interrupted"); err != nil {
		t.Fatal(err)
	}
	if err = engine.DB.FinishDeployment(id, "failed", "simulated interruption"); err != nil {
		t.Fatal(err)
	}
	if err = engine.Rollback(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if string(ftp.files["/site/index.html"]) != "previous" {
		t.Fatalf("previous file not restored: %q", ftp.files["/site/index.html"])
	}
	if _, ok := ftp.files["/site/new.js"]; ok {
		t.Fatal("new file remained after rollback")
	}
}

func TestSafeRelativeRejectsTraversal(t *testing.T) {
	for _, s := range []string{"../secret", "/rooted", "C:\\temp", "."} {
		if _, err := safeRelative(s); err == nil {
			t.Errorf("safeRelative(%q) succeeded", s)
		}
	}
	if got, err := safeRelative("assets/app.js"); err != nil || got != path.Clean("assets/app.js") {
		t.Fatalf("got %q, %v", got, err)
	}
}
