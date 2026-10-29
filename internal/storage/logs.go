package storage

import "time"

type DeploymentLog struct {
	ID        int64     `json:"id"`
	Stream    string    `json:"stream"`
	Line      string    `json:"line"`
	CreatedAt time.Time `json:"createdAt"`
}
type DeploymentLogPage struct {
	Entries []DeploymentLog `json:"entries"`
	Total   int             `json:"total"`
}

func (db *DB) AppendDeploymentLog(id int64, stream, line string) error {
	_, err := db.Conn.Exec(`INSERT INTO deployment_logs(deployment_id,stream,line) VALUES(?,?,?)`, id, stream, line)
	return err
}
func (db *DB) DeploymentLogs(id int64, offset, limit int) (DeploymentLogPage, error) {
	page := DeploymentLogPage{Entries: []DeploymentLog{}}
	if offset < 0 {
		offset = 0
	}
	if limit < 1 || limit > 500 {
		limit = 50
	}
	if err := db.Conn.QueryRow(`SELECT COUNT(*) FROM deployment_logs WHERE deployment_id=?`, id).Scan(&page.Total); err != nil {
		return page, err
	}
	rows, err := db.Conn.Query(`SELECT id,stream,line,created_at FROM deployment_logs WHERE deployment_id=? ORDER BY id LIMIT ? OFFSET ?`, id, limit, offset)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry DeploymentLog
		if err := rows.Scan(&entry.ID, &entry.Stream, &entry.Line, &entry.CreatedAt); err != nil {
			return page, err
		}
		page.Entries = append(page.Entries, entry)
	}
	return page, rows.Err()
}

// ClearDeploymentLogs clears text output only; rollback journals and backups remain intact.
func (db *DB) ClearDeploymentLogs(id int64) error {
	if id == 0 {
		_, err := db.Conn.Exec(`DELETE FROM deployment_logs`)
		return err
	}
	_, err := db.Conn.Exec(`DELETE FROM deployment_logs WHERE deployment_id=?`, id)
	return err
}
