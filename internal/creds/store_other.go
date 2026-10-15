//go:build !windows

package creds

import "sync"

type memoryStore struct {
	mu      sync.RWMutex
	secrets map[string]string
}

func NewStore() Store {
	return &memoryStore{
		secrets: make(map[string]string),
	}
}

func (s *memoryStore) SavePassword(targetKey, username, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets[targetKey] = password
	return nil
}

func (s *memoryStore) GetPassword(targetKey string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.secrets[targetKey]
	if !ok {
		return "", ErrCredentialNotFound
	}
	return val, nil
}

func (s *memoryStore) DeletePassword(targetKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.secrets, targetKey)
	return nil
}
