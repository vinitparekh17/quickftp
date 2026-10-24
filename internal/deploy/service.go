// Package deploy connects saved profiles, project handlers, the FTP client, and journaled engine.
package deploy

import (
	"context"
	"fmt"
	"time"

	"quickftp/internal/creds"
	"quickftp/internal/projecthandler"
	"quickftp/internal/storage"
	"quickftp/internal/transfer"
)

type Service struct {
	DB           *storage.DB
	Credentials  creds.Store
	BackupRoot   string
	BuildLog     projecthandler.LogFunc
	PhaseLog     func(string)
	TransferLog  func(string, string)
	Gate         *transfer.Gate
	TransferFile func(deploymentID, connectionID int64, connectionName, remote, local string, total, bytes int64, status, detail string)
}

type Options struct {
	IISOfflinePage   string
	IISShutdownDelay time.Duration
}

func (s *Service) DeployProject(ctx context.Context, projectName, connectionName string, opts Options) (int64, error) {
	if s == nil || s.DB == nil || s.Credentials == nil {
		return 0, fmt.Errorf("deployment service requires database and credential store")
	}
	project, err := s.DB.GetProjectByName(projectName)
	if err != nil {
		return 0, err
	}
	connection, err := s.DB.GetConnectionByName(connectionName)
	if err != nil {
		return 0, err
	}
	password, err := s.Credentials.GetPassword(connection.CredTargetKey)
	if err != nil {
		return 0, fmt.Errorf("read connection credential: %w", err)
	}
	handler, err := projecthandler.ForType(project.Type)
	if err != nil {
		return 0, err
	}
	if s.PhaseLog != nil {
		s.PhaseLog("build")
	}
	artifacts, err := handler.Prepare(ctx, projecthandler.Project{Type: project.Type, LocalPath: project.LocalPath, BuildCommand: project.BuildCommand, OutputRelPath: project.OutputRelPath, ArtifactRulesJSON: project.ArtifactRulesJSON}, s.BuildLog)
	if err != nil {
		return 0, err
	}
	selected := make([]transfer.Artifact, len(artifacts))
	for i, a := range artifacts {
		selected[i] = transfer.Artifact{LocalPath: a.LocalPath, RelativePath: a.RelativePath}
	}
	client := &transfer.FTPClient{Config: transfer.FTPConfig{Host: connection.Host, Port: connection.Port, Protocol: connection.Protocol, Username: connection.Username, Password: password, TLSVerify: connection.TLSVerify, ReuseConnection: true}}
	defer client.Close()
	if s.PhaseLog != nil {
		s.PhaseLog("planning")
	}
	backupRoot := s.BackupRoot
	if backupRoot == "" {
		paths, err := storage.InitAppPaths()
		if err != nil {
			return 0, err
		}
		backupRoot = paths.BackupsDir
	}
	engine := transfer.Engine{DB: s.DB, Client: client, RemoteRoot: connection.RemoteRoot, BackupRoot: backupRoot, IISOfflinePage: opts.IISOfflinePage, IISShutdownDelay: opts.IISShutdownDelay, OnPhase: s.PhaseLog, OnLog: s.TransferLog, Gate: s.Gate, ConnectionID: connection.ID, ConnectionName: connection.Name, OnFileTransfer: s.TransferFile}
	return engine.Deploy(ctx, project.ID, connection.ID, selected)
}

func (s *Service) Rollback(ctx context.Context, deploymentID int64, connectionName string) error {
	if s == nil || s.DB == nil || s.Credentials == nil {
		return fmt.Errorf("deployment service requires database and credential store")
	}
	d, err := s.DB.GetDeployment(deploymentID)
	if err != nil {
		return err
	}
	connection, err := s.DB.GetConnectionByName(connectionName)
	if err != nil {
		return err
	}
	if d.ConnectionID != connection.ID {
		return fmt.Errorf("deployment %d belongs to a different connection", deploymentID)
	}
	password, err := s.Credentials.GetPassword(connection.CredTargetKey)
	if err != nil {
		return fmt.Errorf("read connection credential: %w", err)
	}
	client := &transfer.FTPClient{Config: transfer.FTPConfig{Host: connection.Host, Port: connection.Port, Protocol: connection.Protocol, Username: connection.Username, Password: password, TLSVerify: connection.TLSVerify, ReuseConnection: true}}
	defer client.Close()
	return (&transfer.Engine{DB: s.DB, Client: client, RemoteRoot: connection.RemoteRoot, Gate: s.Gate, ConnectionID: connection.ID, ConnectionName: connection.Name, OnFileTransfer: s.TransferFile}).Rollback(ctx, deploymentID)
}
