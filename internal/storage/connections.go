package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Protocol string

const (
	ProtocolFTP          Protocol = "ftp"
	ProtocolFTPSExplicit Protocol = "ftps_explicit"
)

type Connection struct {
	ID            int64
	Name          string
	Host          string
	Port          int
	Protocol      Protocol
	Username      string
	RemoteRoot    string
	TLSVerify     bool
	CredTargetKey string
	CreatedAt     time.Time
}

func (db *DB) UpsertConnection(c *Connection) error {
	tlsVerifyInt := 0
	if c.TLSVerify {
		tlsVerifyInt = 1
	}

	query := `
	INSERT INTO connections (name, host, port, protocol, username, remote_root, tls_verify, cred_target_key)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(name) DO UPDATE SET
		host = excluded.host,
		port = excluded.port,
		protocol = excluded.protocol,
		username = excluded.username,
		remote_root = excluded.remote_root,
		tls_verify = excluded.tls_verify,
		cred_target_key = excluded.cred_target_key
	RETURNING id, created_at;
	`

	return db.Conn.QueryRow(
		query,
		c.Name,
		c.Host,
		c.Port,
		string(c.Protocol),
		c.Username,
		c.RemoteRoot,
		tlsVerifyInt,
		c.CredTargetKey,
	).Scan(&c.ID, &c.CreatedAt)
}

func (db *DB) GetConnectionByName(name string) (*Connection, error) {
	query := `
	SELECT id, name, host, port, protocol, username, remote_root, tls_verify, cred_target_key, created_at
	FROM connections
	WHERE name = ?;
	`

	var c Connection
	var protocolStr string
	var tlsVerifyInt int

	err := db.Conn.QueryRow(query, name).Scan(
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
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("connection %q not found", name)
		}
		return nil, err
	}

	c.Protocol = Protocol(protocolStr)
	c.TLSVerify = tlsVerifyInt == 1
	return &c, nil
}
