package creds

import (
	"errors"
	"fmt"
)

var ErrCredentialNotFound = errors.New("credential not found")

// Store abstracts secret management so tests and non-Windows environments work seamlessly.
type Store interface {
	SavePassword(targetKey, username, password string) error
	GetPassword(targetKey string) (string, error)
	DeletePassword(targetKey string) error
}

// FormatTargetKey creates a consistent key for the connections.cred_target_key column.
func FormatTargetKey(connectionName string) string {
	return fmt.Sprintf("QuickFTP/%s", connectionName)
}
