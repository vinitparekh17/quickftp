package storage

import (
	"path/filepath"
	"testing"
)

func TestDeploymentLogsStayScopedAndClearingPreservesJournal(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "logs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Conn.Exec(`INSERT INTO projects(name,project_type,local_path,build_cmd,output_rel_path) VALUES('p','dotnet','.','','auto'); INSERT INTO connections(name,host,protocol,username,cred_target_key) VALUES('c','localhost','ftp','test','test')`); err != nil {
		t.Fatal(err)
	}
	first, err := db.StartDeployment(1, 1, "backup")
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.StartDeployment(1, 1, "backup")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Conn.Exec(`INSERT INTO projects(name,project_type,local_path,build_cmd,output_rel_path) VALUES('other','dotnet','.','','auto')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60; i++ {
		if _, err := db.StartDeployment(2, 1, ""); err != nil {
			t.Fatal(err)
		}
	}
	scoped, err := db.ListDeployments(1, 1)
	if err != nil || len(scoped) != 1 || scoped[0].ID != second {
		t.Fatalf("project history was displaced by other projects: %+v, %v", scoped, err)
	}

	for i := 0; i < 55; i++ {
		if err := db.AppendDeploymentLog(first, "stdout", "build output"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AppendDeploymentLog(second, "stderr", "other run failure"); err != nil {
		t.Fatal(err)
	}
	if err := db.JournalFile(first, "app.dll", "backup", "verified", 3, "sha", ""); err != nil {
		t.Fatal(err)
	}
	page, err := db.DeploymentLogs(first, 50, 50)
	if err != nil || page.Total != 55 || len(page.Entries) != 5 {
		t.Fatalf("pagination: %+v, %v", page, err)
	}
	if err := db.ClearDeploymentLogs(first); err != nil {
		t.Fatal(err)
	}
	other, err := db.DeploymentLogs(second, 0, 50)
	if err != nil || other.Total != 1 {
		t.Fatalf("other run logs changed: %+v, %v", other, err)
	}
	files, err := db.GetDeploymentFiles(first, "")
	if err != nil || len(files) != 1 {
		t.Fatalf("clearing logs deleted rollback journal: %+v, %v", files, err)
	}
	if err := db.ClearDeploymentLogs(0); err != nil {
		t.Fatal(err)
	}
	other, _ = db.DeploymentLogs(second, 0, 50)
	if other.Total != 0 {
		t.Fatal("clear all text logs left entries")
	}
}
