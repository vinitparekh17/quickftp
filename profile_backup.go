package main

import (
	"encoding/json"
	"fmt"
)

func profileBackupFormat(kind string) (string, error) {
	switch kind {
	case "project", "connection":
		return "quickftp-" + kind, nil
	default:
		return "", fmt.Errorf("unsupported profile type")
	}
}

func (a *App) profileBackup(kind string, id int64) ([]byte, error) {
	format, err := profileBackupFormat(kind)
	if err != nil {
		return nil, err
	}
	backup := DataBackup{Format: format, Version: 1}
	if kind == "connection" {
		c, err := a.connectionByID(id)
		if err != nil {
			return nil, err
		}
		backup.Connections = []BackupConnection{backupConnection(connectionToDTO(*c))}
	} else {
		projects, err := a.GetProjects()
		if err != nil {
			return nil, err
		}
		for _, project := range projects {
			if project.ID == id {
				project.ID = 0
				backup.Projects = []ProjectDTO{project}
				break
			}
		}
		if len(backup.Projects) == 0 {
			return nil, fmt.Errorf("project %d not found", id)
		}
	}
	return json.MarshalIndent(backup, "", "  ")
}

// ExportProfile saves one project or connection without app settings or credentials.
func (a *App) ExportProfile(kind string, id int64) (bool, error) {
	data, err := a.profileBackup(kind, id)
	if err != nil {
		return false, err
	}
	return a.saveBackupFile("Export "+kind, "quickftp-"+kind+".json", data)
}

// ImportProfile creates one profile, skipping an existing name.
func (a *App) ImportProfile(kind string) (*ImportResult, error) {
	format, err := profileBackupFormat(kind)
	if err != nil {
		return nil, err
	}
	raw, err := a.openBackupFile("Import " + kind)
	if err != nil || raw == nil {
		return nil, err
	}
	backup, err := decodeBackupFormat(raw, format)
	if err != nil {
		return nil, err
	}
	return a.applyBackup(backup)
}
