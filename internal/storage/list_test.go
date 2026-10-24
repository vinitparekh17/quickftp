package storage

import (
	"path/filepath"
	"testing"
)

func TestStorageListAndCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	// 1. Test Connection List & Delete
	c1 := &Connection{
		Name:          "Conn-A",
		Host:          "ftp.example.com",
		Port:          21,
		Protocol:      ProtocolFTP,
		Username:      "userA",
		RemoteRoot:    "/",
		TLSVerify:     false,
		CredTargetKey: "QuickFTP/Conn-A",
	}
	if err := db.UpsertConnection(c1); err != nil {
		t.Fatalf("UpsertConnection failed: %v", err)
	}

	conns, err := db.ListConnections()
	if err != nil {
		t.Fatalf("ListConnections failed: %v", err)
	}
	if len(conns) != 1 || conns[0].Name != "Conn-A" {
		t.Fatalf("unexpected conns: %+v", conns)
	}

	if err := db.DeleteConnection(c1.ID); err != nil {
		t.Fatalf("DeleteConnection failed: %v", err)
	}
	connsAfter, err := db.ListConnections()
	if err != nil {
		t.Fatalf("ListConnections after delete failed: %v", err)
	}
	if len(connsAfter) != 0 {
		t.Fatalf("expected 0 conns after delete, got %d", len(connsAfter))
	}

	// 2. Test Project List & Delete
	p1 := &Project{
		Name:          "Project-1",
		Type:          "angular",
		LocalPath:     "/test/path",
		BuildCommand:  "npm run build",
		OutputRelPath: "dist",
	}
	if err := db.UpsertProject(p1); err != nil {
		t.Fatalf("UpsertProject failed: %v", err)
	}

	projs, err := db.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(projs) != 1 || projs[0].Name != "Project-1" {
		t.Fatalf("unexpected projs: %+v", projs)
	}

	if err := db.DeleteProject(p1.ID); err != nil {
		t.Fatalf("DeleteProject failed: %v", err)
	}
	projsAfter, err := db.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects after delete failed: %v", err)
	}
	if len(projsAfter) != 0 {
		t.Fatalf("expected 0 projs after delete, got %d", len(projsAfter))
	}

	// 3. Test ListDeployments, GetDeployment, and JournalFiles
	if err := db.UpsertConnection(c1); err != nil {
		t.Fatalf("UpsertConnection re-add failed: %v", err)
	}
	if err := db.UpsertProject(p1); err != nil {
		t.Fatalf("UpsertProject re-add failed: %v", err)
	}

	depID, err := db.StartDeployment(p1.ID, c1.ID, "/tmp/backup")
	if err != nil {
		t.Fatalf("StartDeployment failed: %v", err)
	}

	deps, err := db.ListDeployments(10)
	if err != nil {
		t.Fatalf("ListDeployments failed: %v", err)
	}
	if len(deps) != 1 || deps[0].ID != depID {
		t.Fatalf("unexpected deployments: %+v", deps)
	}

	// 4. Test GetDeployment
	singleDep, err := db.GetDeployment(depID)
	if err != nil {
		t.Fatalf("GetDeployment failed: %v", err)
	}
	if singleDep.ID != depID || singleDep.Status != "running" || singleDep.CurrentPhase != "planning" {
		t.Fatalf("unexpected singleDep: %+v", singleDep)
	}

	// 5. Test JournalFile and GetDeploymentFiles (both filtered by stage and all stages when stage="")
	if err := db.JournalFile(depID, "file1.txt", "upload", "verified", 100, "hash1", ""); err != nil {
		t.Fatalf("JournalFile upload failed: %v", err)
	}
	if err := db.JournalFile(depID, "file2.bak", "backup", "verified", 200, "hash2", ""); err != nil {
		t.Fatalf("JournalFile backup failed: %v", err)
	}
	if err := db.JournalFile(depID, "file3.old", "delete", "completed", 300, "", ""); err != nil {
		t.Fatalf("JournalFile delete failed: %v", err)
	}

	uploads, err := db.GetDeploymentFiles(depID, "upload")
	if err != nil {
		t.Fatalf("GetDeploymentFiles(upload) failed: %v", err)
	}
	if len(uploads) != 1 || uploads[0].RelativePath != "file1.txt" {
		t.Fatalf("unexpected uploads: %+v", uploads)
	}

	allFiles, err := db.GetDeploymentFiles(depID, "")
	if err != nil {
		t.Fatalf("GetDeploymentFiles(all) failed: %v", err)
	}
	if len(allFiles) != 3 {
		t.Fatalf("expected 3 allFiles, got %d", len(allFiles))
	}
}
