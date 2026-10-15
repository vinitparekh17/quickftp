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

	// 3. Test ListDeployments
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
}

