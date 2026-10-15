package main

import (
	"log/slog"
	"os"

	"quickftp/internal/creds"
	"quickftp/internal/storage"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	paths, err := storage.InitAppPaths()
	if err != nil {
		logger.Error("Failed to initialize app paths", "err", err)
		os.Exit(1)
	}

	db, err := storage.OpenDB(paths.DBPath)
	if err != nil {
		logger.Error("Failed to initialize SQLite database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	credStore := creds.NewStore()

	// 1. Create a test connection profile
	connName := "Staging-IIS"
	targetKey := creds.FormatTargetKey(connName)

	conn := &storage.Connection{
		Name:          connName,
		Host:          "ftp.example.com",
		Port:          21,
		Protocol:      storage.ProtocolFTPSExplicit,
		Username:      "deploy_user",
		RemoteRoot:    "/site/wwwroot",
		TLSVerify:     true,
		CredTargetKey: targetKey,
	}

	if err := db.UpsertConnection(conn); err != nil {
		logger.Error("Failed to save connection to SQLite", "err", err)
		os.Exit(1)
	}

	// 2. Store the secret in Credential Manager
	if err := credStore.SavePassword(targetKey, conn.Username, "super-secret-password-123"); err != nil {
		logger.Error("Failed to save password to credential store", "err", err)
		os.Exit(1)
	}

	// 3. Read back both
	savedConn, err := db.GetConnectionByName(connName)
	if err != nil {
		logger.Error("Failed to load connection from SQLite", "err", err)
		os.Exit(1)
	}

	secret, err := credStore.GetPassword(savedConn.CredTargetKey)
	if err != nil {
		logger.Error("Failed to read password from credential store", "err", err)
		os.Exit(1)
	}

	logger.Info("Step 2 verified successfully",
		"conn_id", savedConn.ID,
		"host", savedConn.Host,
		"cred_key", savedConn.CredTargetKey,
		"secret_length", len(secret),
	)

	// 4. Cleanup test credential
	_ = credStore.DeletePassword(targetKey)
}