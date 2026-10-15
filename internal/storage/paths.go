package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

type AppPaths struct {
	RootDir    string
	DBPath     string
	BackupsDir string
	LogsDir    string
}

// InitAppPaths ensures all required local directories exist on startup.
func InitAppPaths() (*AppPaths, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve user config dir: %w", err)
	}

	root := filepath.Join(configDir, "QuickFTP")
	backups := filepath.Join(root, "backups")
	logs := filepath.Join(root, "logs")

	for _, dir := range []string{root, backups, logs} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return &AppPaths{
		RootDir:    root,
		DBPath:     filepath.Join(root, "quickftp.db"),
		BackupsDir: backups,
		LogsDir:    logs,
	}, nil
}
