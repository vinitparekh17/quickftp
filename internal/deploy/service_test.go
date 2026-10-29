package deploy

import (
	"context"
	"path/filepath"
	"quickftp/internal/storage"
	"testing"
)

type testCredentials struct{}

func (testCredentials) GetPassword(string) (string, error)        { return "test", nil }
func (testCredentials) SavePassword(string, string, string) error { return nil }
func (testCredentials) DeletePassword(string) error               { return nil }
func TestBuildFailureHasProjectDeploymentRecordAndLogs(t *testing.T) {
	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "service.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project := &storage.Project{Name: "p", Type: "angular", LocalPath: t.TempDir(), BuildCommand: "echo build-diagnostic && exit 1", OutputRelPath: "dist"}
	if err := db.UpsertProject(project); err != nil {
		t.Fatal(err)
	}
	connection := &storage.Connection{Name: "c", Host: "unused.invalid", Port: 21, Protocol: storage.ProtocolFTP, Username: "test", RemoteRoot: "/", CredTargetKey: "test"}
	if err := db.UpsertConnection(connection); err != nil {
		t.Fatal(err)
	}
	var started int64
	service := Service{DB: db, Credentials: testCredentials{}, BackupRoot: t.TempDir(), DeploymentStarted: func(id int64) { started = id }}
	service.BuildLog = func(stream, line string) {
		if started == 0 {
			t.Error("build ran before durable record")
		}
		if err := db.AppendDeploymentLog(started, stream, line); err != nil {
			t.Error(err)
		}
	}
	id, err := service.DeployProject(context.Background(), "p", "c", Options{})
	if err == nil || id == 0 || id != started {
		t.Fatalf("build failure missing run identity: %d, %d, %v", id, started, err)
	}
	record, err := db.GetDeployment(id)
	if err != nil || record.ProjectID != project.ID || record.Status != "failed" || record.BackupDir != "" {
		t.Fatalf("invalid failed build record: %+v, %v", record, err)
	}
	logs, err := db.DeploymentLogs(id, 0, 50)
	if err != nil || logs.Total == 0 {
		t.Fatalf("build diagnostics not retained: %+v, %v", logs, err)
	}
}
