package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"quickftp/internal/creds"
)

// BackupConnection deliberately has no password or machine-specific credential key.
type BackupConnection struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Protocol   string `json:"protocol"`
	Username   string `json:"username"`
	RemoteRoot string `json:"remoteRoot"`
	TLSVerify  bool   `json:"tlsVerify"`
}

// UnmarshalJSON preserves an explicit opt-out and verifies certificates by default.
func (c *BackupConnection) UnmarshalJSON(raw []byte) error {
	type fields BackupConnection
	value := fields{TLSVerify: true}
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	*c = BackupConnection(value)
	return nil
}

type DataBackup struct {
	Format        string             `json:"format"`
	Version       int                `json:"version"`
	Connections   []BackupConnection `json:"connections"`
	Projects      []ProjectDTO       `json:"projects"`
	Configuration *SettingsDTO       `json:"configuration,omitempty"`
}

type ImportResult struct {
	Connections int `json:"connections"`
	Projects    int `json:"projects"`
	Skipped     int `json:"skipped"`
}

func backupConnection(c ConnectionDTO) BackupConnection {
	return BackupConnection{c.Name, c.Host, c.Port, c.Protocol, c.Username, c.RemoteRoot, c.TLSVerify}
}

func (a *App) backupData() ([]byte, error) {
	connections, err := a.GetConnections()
	if err != nil {
		return nil, err
	}
	projects, err := a.GetProjects()
	if err != nil {
		return nil, err
	}
	settings := a.GetSettings()
	backup := DataBackup{Format: "quickftp", Version: 1, Connections: make([]BackupConnection, 0, len(connections)), Projects: projects, Configuration: &settings}
	for _, c := range connections {
		backup.Connections = append(backup.Connections, backupConnection(c))
	}
	for i := range backup.Projects {
		backup.Projects[i].ID = 0
	}
	return json.MarshalIndent(backup, "", "  ")
}

// ExportData uses a native file dialog; cancellation returns false without error.
func (a *App) ExportData() (bool, error) {
	data, err := a.backupData()
	if err != nil {
		return false, err
	}
	return a.saveBackupFile("Export QuickFTP data", "quickftp-backup.json", data)
}

func (a *App) saveBackupFile(title, defaultName string, data []byte) (bool, error) {
	filename, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{Title: title, DefaultFilename: defaultName, Filters: []wailsRuntime.FileFilter{{DisplayName: "QuickFTP backup (*.json)", Pattern: "*.json"}}})
	if err != nil || filename == "" {
		return false, err
	}
	err = writePrivateFile(filename, data)
	return err == nil, err
}

func decodeBackup(raw []byte) (*DataBackup, error) {
	return decodeBackupFormat(raw, "quickftp")
}

func decodeBackupFormat(raw []byte, format string) (*DataBackup, error) {
	var b DataBackup
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("invalid backup JSON: %w", err)
	}
	if b.Format != format || b.Version != 1 {
		return nil, fmt.Errorf("unsupported QuickFTP backup format or version")
	}
	switch format {
	case "quickftp-project":
		if len(b.Projects) != 1 || len(b.Connections) != 0 || b.Configuration != nil {
			return nil, fmt.Errorf("expected a backup containing exactly one project")
		}
	case "quickftp-connection":
		if len(b.Connections) != 1 || len(b.Projects) != 0 || b.Configuration != nil {
			return nil, fmt.Errorf("expected a backup containing exactly one connection")
		}
	default:
		if b.Connections == nil || b.Projects == nil || b.Configuration == nil {
			return nil, fmt.Errorf("backup must include connections, projects and configuration")
		}
	}
	names := map[string]bool{}
	for _, c := range b.Connections {
		if strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Host) == "" || strings.TrimSpace(c.Username) == "" || c.Port < 1 || c.Port > 65535 || (c.Protocol != "ftp" && c.Protocol != "ftps_explicit") {
			return nil, fmt.Errorf("invalid connection profile")
		}
		if names[c.Name] {
			return nil, fmt.Errorf("duplicate connection name in backup")
		}
		names[c.Name] = true
	}
	names = map[string]bool{}
	for _, p := range b.Projects {
		if strings.TrimSpace(p.Name) == "" || (p.ProjectType != "angular" && p.ProjectType != "dotnet") || strings.TrimSpace(p.LocalPath) == "" || strings.TrimSpace(p.BuildCommand) == "" || strings.TrimSpace(p.OutputRelPath) == "" {
			return nil, fmt.Errorf("invalid project profile")
		}
		if p.ArtifactRulesJSON != "" && !json.Valid([]byte(p.ArtifactRulesJSON)) {
			return nil, fmt.Errorf("invalid artifact rules JSON")
		}
		if names[p.Name] {
			return nil, fmt.Errorf("duplicate project name in backup")
		}
		names[p.Name] = true
	}
	s := b.Configuration
	if s == nil {
		return &b, nil
	}
	if strings.TrimSpace(s.BackupDir) == "" || strings.TrimSpace(s.DownloadDir) == "" || s.MaxConcurrent < 1 || s.MaxConcurrent > 5 {
		return nil, fmt.Errorf("invalid configuration")
	}
	for _, policy := range []string{s.DownloadConflict, s.UploadConflict} {
		if policy != "Ask" && policy != "Skip" && policy != "Rename" && policy != "Overwrite" {
			return nil, fmt.Errorf("invalid collision policy")
		}
	}
	return &b, nil
}

func (a *App) importData(raw []byte) (*ImportResult, error) {
	b, err := decodeBackup(raw)
	if err != nil {
		return nil, err
	}
	return a.applyBackup(b)
}

func (a *App) applyBackup(b *DataBackup) (*ImportResult, error) {
	a.deployMu.Lock()
	defer a.deployMu.Unlock()
	if a.deployCancel != nil {
		return nil, fmt.Errorf("wait for the active deployment to finish before importing")
	}
	// Keep existing IDs, history and credentials. Only new names are inserted.
	tx, err := a.db.Conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result := &ImportResult{}
	for _, c := range b.Connections {
		res, e := tx.Exec(`INSERT INTO connections(name,host,port,protocol,username,remote_root,tls_verify,cred_target_key) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(name) DO NOTHING`, c.Name, c.Host, c.Port, c.Protocol, c.Username, c.RemoteRoot, c.TLSVerify, creds.NewTargetKey())
		if e != nil {
			return nil, fmt.Errorf("import connection: %w", e)
		}
		n, e := res.RowsAffected()
		if e != nil {
			return nil, e
		}
		result.Connections += int(n)
		result.Skipped += 1 - int(n)
	}
	for _, p := range b.Projects {
		if p.ArtifactRulesJSON == "" {
			p.ArtifactRulesJSON = "{}"
		}
		res, e := tx.Exec(`INSERT INTO projects(name,project_type,local_path,build_cmd,output_rel_path,artifact_rules_json) VALUES(?,?,?,?,?,?) ON CONFLICT(name) DO NOTHING`, p.Name, p.ProjectType, p.LocalPath, p.BuildCommand, p.OutputRelPath, p.ArtifactRulesJSON)
		if e != nil {
			return nil, fmt.Errorf("import project: %w", e)
		}
		n, e := res.RowsAffected()
		if e != nil {
			return nil, e
		}
		result.Projects += int(n)
		result.Skipped += 1 - int(n)
	}
	if b.Configuration == nil {
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return result, nil
	}
	old := a.settings
	old.BackupDir = a.service.BackupRoot
	if err = a.saveSettingsLocked(*b.Configuration); err != nil {
		return nil, fmt.Errorf("import configuration: %w", err)
	}
	if err = tx.Commit(); err != nil {
		if restoreErr := a.saveSettingsLocked(old); restoreErr != nil {
			return nil, fmt.Errorf("commit import: %v; restore configuration: %w", err, restoreErr)
		}
		return nil, err
	}
	return result, nil
}

// ImportData only reads the file selected by the user. It never runs imported builds.
func (a *App) ImportData() (*ImportResult, error) {
	raw, err := a.openBackupFile("Import QuickFTP data")
	if err != nil || raw == nil {
		return nil, err
	}
	return a.importData(raw)
}

func (a *App) openBackupFile(title string) ([]byte, error) {
	filename, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{Title: title, Filters: []wailsRuntime.FileFilter{{DisplayName: "QuickFTP backup (*.json)", Pattern: "*.json"}}})
	if err != nil || filename == "" {
		return nil, err
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	const limit = 10 * 1024 * 1024
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > limit {
		return nil, fmt.Errorf("backup exceeds 10 MiB")
	}
	return raw, nil
}

// Write through a private temporary file so failed writes leave the previous file intact.
func writePrivateFile(filename string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(filename), ".quickftp-*")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(temp, filename)
}
