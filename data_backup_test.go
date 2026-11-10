package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quickftp/internal/creds"
	"quickftp/internal/deploy"
	"quickftp/internal/storage"
	"quickftp/internal/transfer"
)

func backupTestApp(t *testing.T) *App {
	t.Helper()
	root := t.TempDir()
	db, err := storage.OpenDB(filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	settings := SettingsDTO{BackupDir: filepath.Join(root, "backups"), DownloadDir: filepath.Join(root, "downloads"), MaxConcurrent: 2, UploadConflict: "Ask", DownloadConflict: "Skip"}
	a := &App{db: db, settings: settings, settingsPath: filepath.Join(root, "settings.json"), service: &deploy.Service{BackupRoot: settings.BackupDir}, gate: transfer.NewGate(2)}
	if err = a.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	return a
}

func sampleBackup(a *App) DataBackup {
	s := a.GetSettings()
	return DataBackup{Format: "quickftp", Version: 1, Connections: []BackupConnection{{Name: "server", Host: "example.com", Port: 21, Protocol: "ftps_explicit", Username: "deploy", RemoteRoot: "/", TLSVerify: true}}, Projects: []ProjectDTO{{Name: "site", ProjectType: "angular", LocalPath: "/projects/site", BuildCommand: "npm run build", OutputRelPath: "dist", ArtifactRulesJSON: `{"exclude":["*.map"]}`}}, Configuration: &s}
}

func TestBackupRoundTripAndMerge(t *testing.T) {
	a := backupTestApp(t)
	b := sampleBackup(a)
	raw, _ := json.Marshal(b)
	result, err := a.importData(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Connections != 1 || result.Projects != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	c, err := a.db.GetConnectionByName("server")
	if err != nil {
		t.Fatal(err)
	}
	if c.CredTargetKey != creds.FormatTargetKey("server") {
		t.Fatal("missing local credential identity")
	}
	raw, err = a.backupData()
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"password", "cred_target", c.CredTargetKey} {
		if strings.Contains(strings.ToLower(string(raw)), strings.ToLower(secret)) {
			t.Fatalf("export includes %s", secret)
		}
	}
	exported, err := decodeBackup(raw)
	if err != nil {
		t.Fatal(err)
	}
	if exported.Projects[0].ID != 0 || exported.Configuration.DownloadConflict != "Skip" || !exported.Connections[0].TLSVerify {
		t.Fatal("round trip lost data")
	}
	exported.Connections[0].Host = "changed.example.com"
	raw, _ = json.Marshal(exported)
	result, err = a.importData(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped != 2 || result.Connections != 0 || result.Projects != 0 {
		t.Fatalf("unexpected merge: %+v", result)
	}
	after, _ := a.db.GetConnectionByName("server")
	if after.ID != c.ID || after.Host != c.Host || after.CredTargetKey != c.CredTargetKey {
		t.Fatal("existing profile changed")
	}
}

func TestInvalidBackupDoesNotImport(t *testing.T) {
	for _, kind := range []string{"version", "missing", "port", "protocol", "policy", "rules", "duplicate", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			a := backupTestApp(t)
			b := sampleBackup(a)
			switch kind {
			case "version":
				b.Version = 2
			case "missing":
				b.Configuration = nil
			case "port":
				b.Connections[0].Port = 0
			case "protocol":
				b.Connections[0].Protocol = "sftp"
			case "policy":
				b.Configuration.UploadConflict = "invalid"
			case "rules":
				b.Projects[0].ArtifactRulesJSON = "{"
			case "duplicate":
				b.Connections = append(b.Connections, b.Connections[0])
			}
			raw, _ := json.Marshal(b)
			if kind == "malformed" {
				raw = []byte("{")
			}
			if _, err := a.importData(raw); err == nil {
				t.Fatal("expected rejection")
			}
			cs, _ := a.db.ListConnections()
			ps, _ := a.db.ListProjects()
			if len(cs) != 0 || len(ps) != 0 {
				t.Fatal("invalid import changed database")
			}
		})
	}
}

func TestImportRollsBackOnSettingsFailure(t *testing.T) {
	a := backupTestApp(t)
	b := sampleBackup(a)
	raw, _ := json.Marshal(b)
	before := a.GetSettings()
	a.settingsPath = filepath.Join(t.TempDir(), "missing", "settings.json")
	if _, err := a.importData(raw); err == nil {
		t.Fatal("expected write failure")
	}
	cs, _ := a.db.ListConnections()
	ps, _ := a.db.ListProjects()
	if len(cs) != 0 || len(ps) != 0 || a.GetSettings() != before {
		t.Fatal("failed import changed saved data")
	}
}

func TestImportRollsBackOnCredentialKeyConflict(t *testing.T) {
	a := backupTestApp(t)
	// A renamed profile retains its old credential key.
	c := &storage.Connection{Name: "renamed", Host: "old.example.com", Port: 21, Protocol: storage.ProtocolFTPSExplicit, Username: "user", RemoteRoot: "/", TLSVerify: true, CredTargetKey: creds.FormatTargetKey("server")}
	if err := a.db.UpsertConnection(c); err != nil {
		t.Fatal(err)
	}
	b := sampleBackup(a)
	raw, _ := json.Marshal(b)
	settingsBefore, _ := os.ReadFile(a.settingsPath)
	if _, err := a.importData(raw); err == nil {
		t.Fatal("expected key conflict")
	}
	cs, _ := a.db.ListConnections()
	ps, _ := a.db.ListProjects()
	if len(cs) != 1 || len(ps) != 0 {
		t.Fatal("partial import")
	}
	settingsAfter, _ := os.ReadFile(a.settingsPath)
	if string(settingsBefore) != string(settingsAfter) {
		t.Fatal("settings changed")
	}
}

func TestImportAppliesConfigurationAndBlocksActiveDeployment(t *testing.T) {
	a := backupTestApp(t)
	b := sampleBackup(a)
	b.Configuration.MaxConcurrent = 4
	b.Configuration.UploadConflict = "Rename"
	raw, _ := json.Marshal(b)
	a.deployCancel = func() {}
	if _, err := a.importData(raw); err == nil {
		t.Fatal("import allowed during deployment")
	}
	a.deployCancel = nil
	if _, err := a.importData(raw); err != nil {
		t.Fatal(err)
	}
	got := a.GetSettings()
	if got.MaxConcurrent != 4 || got.UploadConflict != "Rename" {
		t.Fatal("configuration not applied")
	}
	saved, err := os.ReadFile(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var persisted SettingsDTO
	if err = json.Unmarshal(saved, &persisted); err != nil || persisted != got {
		t.Fatal("configuration not persisted")
	}
}

func TestEmptyBackupRoundTrip(t *testing.T) {
	a := backupTestApp(t)
	raw, err := a.backupData()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.importData(raw); err != nil {
		t.Fatal(err)
	}
}
