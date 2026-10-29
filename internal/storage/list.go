package storage

import "fmt"

// ListConnections returns all saved connection profiles ordered by name.
func (db *DB) ListConnections() ([]Connection, error) {
	rows, err := db.Conn.Query(`SELECT id, name, host, port, protocol, username, remote_root, tls_verify, cred_target_key, created_at FROM connections ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Connection
	for rows.Next() {
		var c Connection
		var protocolStr string
		var tlsVerifyInt int
		if err := rows.Scan(
			&c.ID,
			&c.Name,
			&c.Host,
			&c.Port,
			&protocolStr,
			&c.Username,
			&c.RemoteRoot,
			&tlsVerifyInt,
			&c.CredTargetKey,
			&c.CreatedAt,
		); err != nil {
			return nil, err
		}
		c.Protocol = Protocol(protocolStr)
		c.TLSVerify = tlsVerifyInt == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteConnection removes a connection profile by id.
func (db *DB) DeleteConnection(id int64) error {
	res, err := db.Conn.Exec(`DELETE FROM connections WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("connection %d not found", id)
	}
	return nil
}

// ListProjects returns all saved project profiles ordered by name.
func (db *DB) ListProjects() ([]Project, error) {
	rows, err := db.Conn.Query(`SELECT id, name, project_type, local_path, build_cmd, output_rel_path, artifact_rules_json FROM projects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.LocalPath, &p.BuildCommand, &p.OutputRelPath, &p.ArtifactRulesJSON); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteProject removes a project profile by id.
func (db *DB) DeleteProject(id int64) error {
	res, err := db.Conn.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("project %d not found", id)
	}
	return nil
}

// ListDeployments returns all deployments regardless of status, newest first, up to limit rows.
func (db *DB) ListDeployments(limit int, projectIDs ...int64) ([]Deployment, error) {
	query := `SELECT id, project_id, connection_id, status, current_phase, backup_dir, error_message, started_at, finished_at FROM deployments`
	args := []any{}
	if len(projectIDs) > 0 {
		query += ` WHERE project_id=?`
		args = append(args, projectIDs[0])
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.Conn.Query(query, args...)
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
