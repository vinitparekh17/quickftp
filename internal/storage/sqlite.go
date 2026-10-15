package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type DB struct {
	Conn *sql.DB
}

func OpenDB(dbPath string) (*DB, error) {
	// _pragma parameters ensure every pooled connection applies WAL and Foreign Keys
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", dbPath)

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	// Restrict to 1 open writer connection to prevent SQLITE_BUSY during rapid file journaling
	conn.SetMaxOpenConns(1)

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping sqlite db: %w", err)
	}

	db := &DB{Conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return db, nil
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS connections (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		name            TEXT NOT NULL UNIQUE,
		host            TEXT NOT NULL,
		port            INTEGER NOT NULL DEFAULT 21,
		protocol        TEXT NOT NULL CHECK(protocol IN ('ftp', 'ftps_explicit')),
		username        TEXT NOT NULL,
		remote_root     TEXT NOT NULL DEFAULT '/',
		tls_verify      INTEGER NOT NULL DEFAULT 1,
		cred_target_key TEXT NOT NULL UNIQUE,
		created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS projects (
		id                  INTEGER PRIMARY KEY AUTOINCREMENT,
		name                TEXT NOT NULL UNIQUE,
		project_type        TEXT NOT NULL CHECK(project_type IN ('angular', 'dotnet')),
		local_path          TEXT NOT NULL,
		build_cmd           TEXT NOT NULL,
		output_rel_path     TEXT NOT NULL,
		artifact_rules_json TEXT NOT NULL DEFAULT '{}',
		created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS deployments (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id      INTEGER NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
		connection_id   INTEGER NOT NULL REFERENCES connections(id) ON DELETE RESTRICT,
		status          TEXT NOT NULL CHECK(status IN ('running', 'completed', 'failed', 'rolled_back')),
		current_phase   TEXT NOT NULL,
		backup_dir      TEXT NOT NULL,
		error_message   TEXT,
		started_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		finished_at     DATETIME
	);

	CREATE TABLE IF NOT EXISTS deployment_files (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		deployment_id   INTEGER NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
		relative_path   TEXT NOT NULL,
		stage           TEXT NOT NULL CHECK(stage IN ('backup', 'delete', 'upload')),
		status          TEXT NOT NULL CHECK(status IN ('pending', 'completed', 'verified', 'failed')),
		size_bytes      INTEGER NOT NULL DEFAULT 0,
		sha256_local    TEXT,
		error_detail    TEXT,
		updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_deployment_files_deployment ON deployment_files(deployment_id);
	`
	_, err := db.Conn.Exec(schema)
	return err
}

func (db *DB) Close() error {
	return db.Conn.Close()
}
