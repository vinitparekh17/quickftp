//go:build windows

package creds

import (
	"errors"
	"fmt"

	"github.com/danieljoos/wincred"
)

type windowsStore struct{}

func NewStore() Store {
	return &windowsStore{}
}

func (s *windowsStore) SavePassword(targetKey, username, password string) error {
	cred := wincred.NewGenericCredential(targetKey)
	cred.UserName = username
	cred.CredentialBlob = []byte(password)
	cred.Persist = wincred.PersistLocalMachine

	if err := cred.Write(); err != nil {
		return fmt.Errorf("wincred write failed for %s: %w", targetKey, err)
	}
	return nil
}

func (s *windowsStore) GetPassword(targetKey string) (string, error) {
	cred, err := wincred.GetGenericCredential(targetKey)
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return "", ErrCredentialNotFound
		}
		return "", fmt.Errorf("wincred read failed for %s: %w", targetKey, err)
	}
	return string(cred.CredentialBlob), nil
}

func (s *windowsStore) DeletePassword(targetKey string) error {
	cred, err := wincred.GetGenericCredential(targetKey)
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return nil // Idempotent delete
		}
		return fmt.Errorf("wincred lookup failed for %s: %w", targetKey, err)
	}
	if err := cred.Delete(); err != nil {
		return fmt.Errorf("wincred delete failed for %s: %w", targetKey, err)
	}
	return nil
}
