package storage

import (
	"database/sql"
	"fmt"
	"time"
)

type Deployment struct {
	ID           int64
	ProjectID    int64
	ConnectionID int64
	Status       string
	CurrentPhase string
	BackupDir    string
	ErrorMessage sql.NullString
	StartedAt    time.Time
	FinishedAt   sql.NullTime
}

type DeploymentFile struct {
	ID           int64
	DeploymentID int64
	RelativePath string
	Stage        string
	Status       string
	SizeBytes    int64
	SHA256Local  sql.NullString
	ErrorDetail  sql.NullString
	UpdatedAt    time.Time
}

// StartDeployment writes the durable deployment record before any remote changes.
func (db *DB) StartDeployment(projectID, connectionID int64, backupDir string) (int64, error) {
	res, err := db.Conn.Exec(`INSERT INTO deployments(project_id, connection_id, status, current_phase, backup_dir) VALUES(?, ?, 'running', 'planning', ?)`, projectID, connectionID, backupDir)
	if err != nil {
		return 0, fmt.Errorf("start deployment: %w", err)
	}
	return res.LastInsertId()
}

func (db *DB) SetDeploymentPhase(id int64, phase string) error {
	_, err := db.Conn.Exec(`UPDATE deployments SET current_phase=? WHERE id=? AND status='running'`, phase, id)
	return err
}

func (db *DB) SetDeploymentBackupDir(id int64, dir string) error {
	_, err := db.Conn.Exec(`UPDATE deployments SET backup_dir=? WHERE id=? AND status='running'`, dir, id)
	return err
}

func (db *DB) FinishDeployment(id int64, status, detail string) error {
	if status != "completed" && status != "failed" && status != "rolled_back" {
		return fmt.Errorf("invalid deployment status %q", status)
	}
	var msg any
	if detail != "" {
		msg = detail
	}
	_, err := db.Conn.Exec(`UPDATE deployments SET status=?, error_message=?, finished_at=CURRENT_TIMESTAMP WHERE id=?`, status, msg, id)
	return err
}

func (db *DB) JournalFile(deploymentID int64, path, stage, status string, size int64, sha, detail string) error {
	if stage != "backup" && stage != "delete" && stage != "upload" {
		return fmt.Errorf("invalid journal stage %q", stage)
	}
	if status != "pending" && status != "completed" && status != "verified" && status != "failed" {
		return fmt.Errorf("invalid journal status %q", status)
	}
	var shaValue, detailValue any
	if sha != "" {
		shaValue = sha
	}
	if detail != "" {
		detailValue = detail
	}
	_, err := db.Conn.Exec(`INSERT INTO deployment_files(deployment_id, relative_path, stage, status, size_bytes, sha256_local, error_detail) VALUES(?, ?, ?, ?, ?, ?, ?)`, deploymentID, path, stage, status, size, shaValue, detailValue)
	return err
}

func (db *DB) PendingDeployments() ([]Deployment, error) {
	rows, err := db.Conn.Query(`SELECT id, project_id, connection_id, status, current_phase, backup_dir, error_message, started_at, finished_at FROM deployments WHERE status='running' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Deployment
	for rows.Next() {
		var d Deployment
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.ConnectionID, &d.Status, &d.CurrentPhase, &d.BackupDir, &d.ErrorMessage, &d.StartedAt, &d.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (db *DB) GetDeployment(id int64) (*Deployment, error) {
	var d Deployment
	err := db.Conn.QueryRow(`SELECT id, project_id, connection_id, status, current_phase, backup_dir, error_message, started_at, finished_at FROM deployments WHERE id=?`, id).Scan(&d.ID, &d.ProjectID, &d.ConnectionID, &d.Status, &d.CurrentPhase, &d.BackupDir, &d.ErrorMessage, &d.StartedAt, &d.FinishedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (db *DB) GetDeploymentFiles(id int64, stage string) ([]DeploymentFile, error) {
	var rows *sql.Rows
	var err error
	if stage != "" {
		rows, err = db.Conn.Query(`SELECT id, deployment_id, relative_path, stage, status, size_bytes, sha256_local, error_detail, updated_at FROM deployment_files WHERE deployment_id=? AND stage=? ORDER BY id`, id, stage)
	} else {
		rows, err = db.Conn.Query(`SELECT id, deployment_id, relative_path, stage, status, size_bytes, sha256_local, error_detail, updated_at FROM deployment_files WHERE deployment_id=? ORDER BY id`, id)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var files []DeploymentFile
	for rows.Next() {
		var f DeploymentFile
		if err := rows.Scan(&f.ID, &f.DeploymentID, &f.RelativePath, &f.Stage, &f.Status, &f.SizeBytes, &f.SHA256Local, &f.ErrorDetail, &f.UpdatedAt); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}
