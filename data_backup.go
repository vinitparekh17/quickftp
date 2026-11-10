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

type DataBackup struct {
	Format        string             `json:"format"`
	Version       int                `json:"version"`
	Connections   []BackupConnection `json:"connections"`
	Projects      []ProjectDTO       `json:"projects"`
	Configuration *SettingsDTO       `json:"configuration"`
}

type ImportResult struct {
	Connections int `json:"connections"`
	Projects    int `json:"projects"`
	Skipped     int `json:"skipped"`
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
		backup.Connections = append(backup.Connections, BackupConnection{c.Name, c.Host, c.Port, c.Protocol, c.Username, c.RemoteRoot, c.TLSVerify})
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
	filename, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{Title: "Export QuickFTP data", DefaultFilename: "quickftp-backup.json", Filters: []wailsRuntime.FileFilter{{DisplayName: "QuickFTP backup (*.json)", Pattern: "*.json"}}})
	if err != nil || filename == "" {
		return false, err
	}
	err = writePrivateFile(filename, data)
	return err == nil, err
}

func decodeBackup(raw []byte) (*DataBackup, error) {
	var b DataBackup
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("invalid backup JSON: %w", err)
	}
	if b.Format != "quickftp" || b.Version != 1 {
		return nil, fmt.Errorf("unsupported QuickFTP backup format or version")
	}
	if b.Connections == nil || b.Projects == nil || b.Configuration == nil {
		return nil, fmt.Errorf("backup must include connections, projects and configuration")
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
		res, e := tx.Exec(`INSERT INTO connections(name,host,port,protocol,username,remote_root,tls_verify,cred_target_key) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(name) DO NOTHING`, c.Name, c.Host, c.Port, c.Protocol, c.Username, c.RemoteRoot, c.TLSVerify, creds.FormatTargetKey(c.Name))
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
	filename, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{Title: "Import QuickFTP data", Filters: []wailsRuntime.FileFilter{{DisplayName: "QuickFTP backup (*.json)", Pattern: "*.json"}}})
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
	return a.importData(raw)
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
