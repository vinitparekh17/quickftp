package transfer

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Recovery rows are committed before a job starts. The ordinal preserves
// queue history order across updates without renumbering existing transfers.
const recoverySchema = `
CREATE TABLE IF NOT EXISTS transfer_recovery (
	ordinal INTEGER PRIMARY KEY,
	id TEXT NOT NULL UNIQUE,
	payload BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS transfer_recovery_migration (
	id INTEGER PRIMARY KEY CHECK (id = 1)
);`

type activityFile struct {
	Version int        `json:"version"`
	Entries []Activity `json:"entries"`
}

func loadActivityRecovery(db *sql.DB, legacyFile string) ([]Activity, error) {
	if db == nil {
		return nil, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(recoverySchema); err != nil {
		return nil, err
	}

	marker, err := tx.Exec("INSERT INTO transfer_recovery_migration(id) VALUES (1) ON CONFLICT DO NOTHING")
	if err != nil {
		return nil, err
	}
	inserted, err := marker.RowsAffected()
	if err != nil {
		return nil, err
	}
	if inserted > 0 && legacyFile != "" {
		legacy, err := readLegacyRecovery(legacyFile)
		if err != nil {
			return nil, err
		}
		// Legacy snapshots are newest first. Insert oldest first to preserve order.
		for i := len(legacy) - 1; i >= 0; i-- {
			if legacy[i].ID != "" && retainRecovery(legacy[i]) {
				if err := writeRecovery(tx, legacy[i]); err != nil {
					return nil, err
				}
			}
		}
	}

	rows, err := tx.Query("SELECT payload FROM transfer_recovery ORDER BY ordinal DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []Activity
	var interrupted []Activity
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var entry Activity
		if err := json.Unmarshal(payload, &entry); err != nil {
			return nil, err
		}
		if isActiveStatus(entry.Status) {
			entry.Status = "Interrupted"
			entry.Error = "Application closed before this transfer completed"
			interrupted = append(interrupted, entry)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, entry := range interrupted {
		if err := writeRecovery(tx, entry); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return entries, nil
}

func readLegacyRecovery(file string) ([]Activity, error) {
	raw, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		raw, err = os.ReadFile(filepath.Join(filepath.Dir(file), "transfers.json"))
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read legacy transfer recovery: %w", err)
	}
	var snapshot activityFile
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		// Older versions ignored malformed snapshots. Keep the legacy file intact.
		return nil, nil
	}
	return snapshot.Entries, nil
}

func retainRecovery(entry Activity) bool {
	return entry.Direction != "Deployment upload" && (isUnfinishedStatus(entry.Status) || entry.Status == "Failed" || entry.Status == "Cancelled")
}

type recoveryWriter interface {
	Exec(string, ...any) (sql.Result, error)
}

func writeRecovery(db recoveryWriter, entry Activity) error {
	if !retainRecovery(entry) {
		_, err := db.Exec("DELETE FROM transfer_recovery WHERE id = ?", entry.ID)
		return err
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO transfer_recovery(id, payload) VALUES (?, ?)
		ON CONFLICT(id) DO UPDATE SET payload = excluded.payload`, entry.ID, payload)
	return err
}

func (m *ActivityManager) saveRecoveryEntries(entries []Activity) error {
	if m.recoveryDB == nil || len(entries) == 0 {
		return nil
	}
	if len(entries) == 1 {
		return writeRecovery(m.recoveryDB, entries[0])
	}
	tx, err := m.recoveryDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, entry := range entries {
		if err := writeRecovery(tx, entry); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (m *ActivityManager) removeRecoveryEntries(ids []string) error {
	if m.recoveryDB == nil || len(ids) == 0 {
		return nil
	}
	tx, err := m.recoveryDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.Exec("DELETE FROM transfer_recovery WHERE id = ?", id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func cleanInterruptedDownload(entry Activity) {
	if entry.ID == "" || strings.ContainsAny(entry.ID, "/\\:") || !filepath.IsAbs(entry.LocalPath) {
		return
	}
	rootPath := entry.DownloadRoot
	if rootPath == "" {
		rootPath = filepath.Dir(entry.LocalPath)
	}
	rel, err := filepath.Rel(rootPath, entry.LocalPath)
	if err != nil || !filepath.IsLocal(rel) || rel == "." {
		return
	}
	if _, err := safeRelative(filepath.ToSlash(rel)); err != nil {
		return
	}
	root, err := os.OpenRoot(rootPath)
	if err == nil {
		defer root.Close()
		_ = root.Remove(rel + ".quickftp-" + entry.ID + ".partial")
	}
}
