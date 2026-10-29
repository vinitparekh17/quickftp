package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quickftp/internal/creds"
	"quickftp/internal/deploy"
	"quickftp/internal/storage"
	"quickftp/internal/transfer"
)

// ---------------------------------------------------------------------------
// Frontend-friendly DTOs — no sql.NullString / sql.NullTime.
// ---------------------------------------------------------------------------

// ConnectionDTO is the frontend-safe representation of a connection profile.
type ConnectionDTO struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Protocol   string `json:"protocol"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	RemoteRoot string `json:"remoteRoot"`
	TLSVerify  bool   `json:"tlsVerify"`
	CreatedAt  string `json:"createdAt"`
}

// ProjectDTO is the frontend-safe representation of a project profile.
type ProjectDTO struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	ProjectType       string `json:"projectType"`
	LocalPath         string `json:"localPath"`
	BuildCommand      string `json:"buildCommand"`
	OutputRelPath     string `json:"outputRelPath"`
	ArtifactRulesJSON string `json:"artifactRulesJson"`
}

// DeploymentDTO is the frontend-safe representation of a deployment record.
type DeploymentDTO struct {
	ID             int64  `json:"id"`
	ProjectID      int64  `json:"projectId"`
	ProjectName    string `json:"projectName"`
	ProjectType    string `json:"projectType"`
	ConnectionID   int64  `json:"connectionId"`
	ConnectionName string `json:"connectionName"`
	ConnectionHost string `json:"connectionHost"`
	RemoteRoot     string `json:"remoteRoot"`
	BuildCommand   string `json:"buildCommand"`
	OutputRelPath  string `json:"outputRelPath"`
	Status         string `json:"status"`
	CurrentPhase   string `json:"currentPhase"`
	BackupDir      string `json:"backupDir"`
	ErrorMessage   string `json:"errorMessage"`
	StartedAt      string `json:"startedAt"`
	FinishedAt     string `json:"finishedAt"`
	Cancelled      bool   `json:"cancelled"`
}

// DeploymentFileDTO is the frontend-safe representation of a deployment file journal entry.
type DeploymentFileDTO struct {
	ID           int64  `json:"id"`
	DeploymentID int64  `json:"deploymentId"`
	RelativePath string `json:"relativePath"`
	Stage        string `json:"stage"`
	Status       string `json:"status"`
	SizeBytes    int64  `json:"sizeBytes"`
	SHA256Local  string `json:"sha256Local"`
	ErrorDetail  string `json:"errorDetail"`
	UpdatedAt    string `json:"updatedAt"`
}

// DeployOptions carries optional settings the frontend passes to StartDeploy.
type DeployOptions struct {
	IISOfflinePage   string `json:"iisOfflinePage"`
	IISShutdownDelay int    `json:"iisShutdownDelay"` // milliseconds
}

type SettingsDTO struct {
	BackupDir        string `json:"backupDir"`
	DownloadDir      string `json:"downloadDir"`
	DownloadConflict string `json:"downloadConflict"`
	UploadConflict   string `json:"uploadConflict"`
	MaxConcurrent    int    `json:"maxConcurrent"`
}

// ---------------------------------------------------------------------------
// App struct — Wails binds every exported method.
// ---------------------------------------------------------------------------

// App holds the application state shared with the Wails runtime.
type App struct {
	ctx             context.Context
	appCancel       context.CancelFunc
	deployWorkers   sync.WaitGroup
	closing         bool
	db              *storage.DB
	creds           creds.Store
	service         *deploy.Service
	deployMu        sync.Mutex
	deployCancel    context.CancelFunc
	settingsPath    string
	activity        *transfer.ActivityManager
	gate            *transfer.Gate
	settings        SettingsDTO
	explorerMu      sync.Mutex
	explorerClients map[int64]*transfer.FTPClient
}

// startup is called by Wails after the frontend is ready.
func (a *App) startup(ctx context.Context) {
	ctx, a.appCancel = context.WithCancel(ctx)
	a.ctx = ctx

	paths, err := storage.InitAppPaths()
	if err != nil {
		log.Fatalf("init paths: %v", err)
	}

	db, err := storage.OpenDB(paths.DBPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	a.db = db
	a.creds = creds.NewStore()
	settingsPath := filepath.Join(paths.RootDir, "settings.json")
	backupRoot := paths.BackupsDir
	var settings SettingsDTO
	if raw, readErr := os.ReadFile(settingsPath); readErr == nil {
		if json.Unmarshal(raw, &settings) == nil && settings.BackupDir != "" {
			if abs, absErr := filepath.Abs(settings.BackupDir); absErr == nil {
				backupRoot = abs
			}
		}
	}
	if settings.MaxConcurrent < 1 || settings.MaxConcurrent > 5 {
		settings.MaxConcurrent = 2
	}
	if settings.DownloadDir == "" {
		if d, e := os.UserHomeDir(); e == nil {
			settings.DownloadDir = filepath.Join(d, "Downloads")
		}
	}
	if abs, e := filepath.Abs(settings.DownloadDir); e == nil {
		settings.DownloadDir = abs
	}
	if settings.DownloadConflict == "" {
		settings.DownloadConflict = "Ask"
	}
	if settings.UploadConflict == "" {
		settings.UploadConflict = "Ask"
	}
	a.settingsPath = settingsPath
	a.settings = settings
	a.gate = transfer.NewGate(settings.MaxConcurrent)
	a.activity = transfer.OpenActivityManager(filepath.Join(paths.RootDir, "transfer-recovery.json"), a.gate)
	a.activity.OnBatchChange = func(values []transfer.Activity) { wailsRuntime.EventsEmit(ctx, "transfer:batch", values) }
	a.activity.OnChange = func(v transfer.Activity) { wailsRuntime.EventsEmit(ctx, "transfer:changed", v) }
	a.activity.OnLog = func(level, line string) {
		wailsRuntime.EventsEmit(ctx, "transfer:log", map[string]string{"level": level, "message": line, "timestamp": time.Now().Format(time.RFC3339)})
	}
	a.service = &deploy.Service{
		DB:          db,
		Credentials: a.creds,
		BackupRoot:  backupRoot,
		Gate:        a.gate,
		TransferFile: func(deploymentID, connectionID int64, connectionName, remote, local string, total, bytes int64, status, detail string) {
			a.activity.RecordDeploymentTransfer(deploymentID, connectionID, connectionName, remote, local, total, bytes, status, detail)
		},
	}
}

func (a *App) GetSettings() SettingsDTO {
	a.deployMu.Lock()
	defer a.deployMu.Unlock()
	s := a.settings
	s.BackupDir = a.service.BackupRoot
	return s
}

func (a *App) SelectDirectory(title string) (string, error) {
	if title == "" {
		title = "Select a directory"
	}
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{Title: title})
}
func (a *App) SelectFiles(title string) ([]string, error) {
	if title == "" {
		title = "Select files to upload"
	}
	return wailsRuntime.OpenMultipleFilesDialog(a.ctx, wailsRuntime.OpenDialogOptions{Title: title})
}

func (a *App) SaveSettings(settings SettingsDTO) error {
	if strings.TrimSpace(settings.BackupDir) == "" {
		return fmt.Errorf("backup directory is required")
	}
	backupDir, err := filepath.Abs(strings.TrimSpace(settings.BackupDir))
	if err != nil {
		return err
	}
	a.deployMu.Lock()
	defer a.deployMu.Unlock()
	if err = os.MkdirAll(backupDir, 0700); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	settings.BackupDir = backupDir
	if settings.MaxConcurrent < 1 || settings.MaxConcurrent > 5 {
		return fmt.Errorf("concurrency must be between 1 and 5")
	}
	if settings.DownloadDir == "" {
		settings.DownloadDir = a.settings.DownloadDir
	}
	settings.DownloadDir, err = filepath.Abs(strings.TrimSpace(settings.DownloadDir))
	if err != nil {
		return err
	}
	if err = os.MkdirAll(settings.DownloadDir, 0700); err != nil {
		return fmt.Errorf("create download directory: %w", err)
	}
	if settings.DownloadConflict == "" {
		settings.DownloadConflict = "Ask"
	}
	if settings.UploadConflict == "" {
		settings.UploadConflict = "Ask"
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(a.settingsPath, data, 0600); err != nil {
		return err
	}
	a.service.BackupRoot = backupDir
	a.settings = settings
	a.gate.SetLimit(settings.MaxConcurrent)
	return nil
}

// shutdown is called by Wails when the application is closing.
func (a *App) shutdown(_ context.Context) {
	a.deployMu.Lock()
	a.closing = true
	if a.appCancel != nil {
		a.appCancel()
	}
	if a.deployCancel != nil {
		a.deployCancel()
	}
	a.deployMu.Unlock()
	if a.activity != nil {
		a.activity.Close()
	}
	a.deployWorkers.Wait()

	a.explorerMu.Lock()
	for id, cl := range a.explorerClients {
		if cl != nil {
			_ = cl.Close()
		}
		delete(a.explorerClients, id)
	}
	a.explorerMu.Unlock()

	if a.db != nil {
		_ = a.db.Close()
	}
}

// ---------------------------------------------------------------------------
// Connection CRUD
// ---------------------------------------------------------------------------

// GetConnections returns all saved connection profiles.
func (a *App) GetConnections() ([]ConnectionDTO, error) {
	conns, err := a.db.ListConnections()
	if err != nil {
		return nil, err
	}
	out := make([]ConnectionDTO, len(conns))
	for i, c := range conns {
		out[i] = connectionToDTO(c)
	}
	return out, nil
}

// SaveConnection upserts a connection profile and persists the password in the OS credential store.
func (a *App) SaveConnection(dto ConnectionDTO) error {
	targetKey := creds.FormatTargetKey(dto.Name)
	if dto.ID > 0 {
		saved, err := a.connectionByID(dto.ID)
		if err != nil {
			return err
		}
		// Keep the credential identity stable across display-name changes.
		targetKey = saved.CredTargetKey
	}
	c := &storage.Connection{
		ID:            dto.ID,
		Name:          dto.Name,
		Host:          dto.Host,
		Port:          dto.Port,
		Protocol:      storage.Protocol(dto.Protocol),
		Username:      dto.Username,
		RemoteRoot:    dto.RemoteRoot,
		TLSVerify:     dto.TLSVerify,
		CredTargetKey: targetKey,
	}
	var saveErr error
	if dto.ID > 0 {
		saveErr = a.db.UpdateConnection(c)
	} else {
		saveErr = a.db.UpsertConnection(c)
	}
	if saveErr != nil {
		return fmt.Errorf("save connection: %w", saveErr)
	}
	if dto.Password != "" {
		if err := a.creds.SavePassword(targetKey, dto.Username, dto.Password); err != nil {
			return fmt.Errorf("save credential: %w", err)
		}
	}
	if dto.ID > 0 {
		a.explorerMu.Lock()
		if a.explorerClients != nil {
			if cl, ok := a.explorerClients[dto.ID]; ok && cl != nil {
				_ = cl.Close()
				delete(a.explorerClients, dto.ID)
			}
		}
		a.explorerMu.Unlock()
	}
	return nil
}

// DeleteConnection removes a connection and its credential from the OS store.
func (a *App) DeleteConnection(id int64) error {
	// Look up the connection first so we can delete the credential.
	conns, err := a.db.ListConnections()
	if err != nil {
		return err
	}
	var targetKey string
	for _, c := range conns {
		if c.ID == id {
			targetKey = c.CredTargetKey
			break
		}
	}
	if err := a.db.DeleteConnection(id); err != nil {
		return err
	}
	a.explorerMu.Lock()
	if a.explorerClients != nil {
		if cl, ok := a.explorerClients[id]; ok && cl != nil {
			_ = cl.Close()
			delete(a.explorerClients, id)
		}
	}
	a.explorerMu.Unlock()
	if targetKey != "" {
		_ = a.creds.DeletePassword(targetKey) // best-effort
	}
	return nil
}

// TestConnection attempts to dial, authenticate, and disconnect.
func (a *App) TestConnection(dto ConnectionDTO) error {
	password := dto.Password
	if password == "" && dto.ID > 0 {
		saved, err := a.connectionByID(dto.ID)
		if err != nil {
			return err
		}
		password, err = a.creds.GetPassword(saved.CredTargetKey)
		if err != nil {
			return fmt.Errorf("read connection credential: %w", err)
		}
	}
	client := &transfer.FTPClient{Config: transfer.FTPConfig{
		Host: dto.Host, Port: dto.Port, Protocol: storage.Protocol(dto.Protocol),
		Username: dto.Username, Password: password, TLSVerify: dto.TLSVerify, Timeout: 15 * time.Second,
	}}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return client.Test(ctx)
}

// ---------------------------------------------------------------------------
// Project CRUD
// ---------------------------------------------------------------------------

// GetProjects returns all saved project profiles.
func (a *App) GetProjects() ([]ProjectDTO, error) {
	projects, err := a.db.ListProjects()
	if err != nil {
		return nil, err
	}
	out := make([]ProjectDTO, len(projects))
	for i, p := range projects {
		out[i] = projectToDTO(p)
	}
	return out, nil
}

// SaveProject upserts a project profile.
func (a *App) SaveProject(dto ProjectDTO) error {
	p := &storage.Project{
		ID:                dto.ID,
		Name:              dto.Name,
		Type:              dto.ProjectType,
		LocalPath:         dto.LocalPath,
		BuildCommand:      dto.BuildCommand,
		OutputRelPath:     dto.OutputRelPath,
		ArtifactRulesJSON: dto.ArtifactRulesJSON,
	}
	return a.db.UpsertProject(p)
}

// DeleteProject removes a project profile by id.
func (a *App) DeleteProject(id int64) error {
	return a.db.DeleteProject(id)
}

// ---------------------------------------------------------------------------
// Deployment operations
// ---------------------------------------------------------------------------

// StartDeploy kicks off a deployment in a background goroutine and emits events.
func (a *App) StartDeploy(projectName, connectionName string, opts DeployOptions) error {
	a.deployMu.Lock()
	if a.closing || a.deployCancel != nil {
		a.deployMu.Unlock()
		return fmt.Errorf("a deployment is already running")
	}
	deployCtx, cancel := context.WithCancel(a.ctx)
	a.deployCancel = cancel
	a.deployWorkers.Add(1)
	service := *a.service
	a.deployMu.Unlock()

	var runID atomic.Int64
	recordLog := func(stream, line string) {
		id := runID.Load()
		if id > 0 {
			if err := a.db.AppendDeploymentLog(id, stream, line); err != nil {
				wailsRuntime.LogError(a.ctx, "Could not persist deployment log: "+err.Error())
			}
		}
		wailsRuntime.EventsEmit(a.ctx, "deploy:log", map[string]interface{}{"deploymentId": id, "stream": stream, "line": line})
	}
	service.BuildLog = recordLog
	service.TransferLog = recordLog
	service.DeploymentStarted = func(id int64) {
		runID.Store(id)
		wailsRuntime.EventsEmit(a.ctx, "deploy:recorded", map[string]interface{}{"deploymentId": id})
	}
	service.PhaseLog = func(phase string) {
		wailsRuntime.EventsEmit(a.ctx, "deploy:phase", map[string]interface{}{"phase": phase})
	}

	deployOpts := deploy.Options{
		IISOfflinePage:   opts.IISOfflinePage,
		IISShutdownDelay: time.Duration(opts.IISShutdownDelay) * time.Millisecond,
	}

	go func() {
		defer a.deployWorkers.Done()
		defer func() {
			cancel()
			a.deployMu.Lock()
			a.deployCancel = nil
			a.deployMu.Unlock()
		}()
		wailsRuntime.EventsEmit(a.ctx, "deploy:started", map[string]string{
			"project":    projectName,
			"connection": connectionName,
		})

		deploymentID, err := service.DeployProject(deployCtx, projectName, connectionName, deployOpts)
		if err != nil {
			recordLog("stderr", err.Error())
			event := "deploy:error"
			if deployCtx.Err() == context.Canceled {
				event = "deploy:cancelled"
			}
			wailsRuntime.EventsEmit(a.ctx, event, map[string]interface{}{
				"deploymentId": deploymentID,
				"error":        err.Error(),
			})
			return
		}

		wailsRuntime.EventsEmit(a.ctx, "deploy:complete", map[string]interface{}{
			"deploymentId": deploymentID,
		})
	}()

	return nil
}

// CancelDeploy requests cancellation of the active deployment.
func (a *App) CancelDeploy() error {
	a.deployMu.Lock()
	defer a.deployMu.Unlock()
	if a.deployCancel == nil {
		return fmt.Errorf("no deployment is running")
	}
	a.deployCancel()
	return nil
}

// RollbackDeployment rolls back a specific deployment.
func (a *App) RollbackDeployment(deploymentID int64, connectionName string) error {
	a.deployMu.Lock()
	if a.closing || a.deployCancel != nil {
		a.deployMu.Unlock()
		return fmt.Errorf("a deployment or rollback is already running")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.deployCancel = cancel
	a.deployWorkers.Add(1)
	service := *a.service
	a.deployMu.Unlock()
	defer a.deployWorkers.Done()
	defer func() {
		cancel()
		a.deployMu.Lock()
		a.deployCancel = nil
		a.deployMu.Unlock()
	}()
	return service.Rollback(ctx, deploymentID, connectionName)
}

func (a *App) ListRemoteDirectory(connectionID int64, remotePath string) ([]transfer.RemoteFile, error) {
	c, err := a.connectionByID(connectionID)
	if err != nil {
		return nil, err
	}
	if err = safeExplorerPath(c.RemoteRoot, remotePath); err != nil {
		return nil, err
	}
	remotePath = path.Clean("/" + remotePath)
	client, err := a.explorerClient(c)
	if err != nil {
		return nil, err
	}
	a.activity.RecordLog("INFO", "Listing "+remotePath)
	files, err := client.ListDirectory(a.ctx, remotePath)
	if err != nil {
		a.activity.RecordLog("ERROR", "Listing "+remotePath+": "+err.Error())
		return nil, err
	}
	a.activity.RecordLog("INFO", fmt.Sprintf("Found %d entries in %s", len(files), remotePath))
	return files, nil
}
func (a *App) QueueDownload(connectionID int64, remotePath, localPath string, size int64) (transfer.Activity, error) {
	settings := a.GetSettings()
	c, err := a.connectionByID(connectionID)
	if err != nil {
		return transfer.Activity{}, err
	}
	if err = safeExplorerPath(c.RemoteRoot, remotePath); err != nil {
		return transfer.Activity{}, err
	}
	localPath, err = safeLocalDestination(settings.DownloadDir, localPath)
	if err != nil {
		return transfer.Activity{}, err
	}
	remotePath = path.Clean("/" + remotePath)
	client, err := a.explorerClient(c)
	if err != nil {
		return transfer.Activity{}, err
	}
	return a.activity.AddDownload(c.ID, c.Name, remotePath, localPath, size, settings.DownloadConflict, client)
}
func (a *App) QueueDownloadDirectory(connectionID int64, remotePath string) ([]transfer.Activity, error) {
	settings := a.GetSettings()
	c, e := a.connectionByID(connectionID)
	if e != nil {
		return nil, e
	}
	if e = safeExplorerPath(c.RemoteRoot, remotePath); e != nil {
		return nil, e
	}
	remotePath = path.Clean("/" + remotePath)
	client, e := a.explorerClient(c)
	if e != nil {
		return nil, e
	}
	root := path.Clean("/" + c.RemoteRoot)
	selectedRel := strings.TrimPrefix(strings.TrimPrefix(remotePath, root), "/")
	selectedDir := filepath.Join(settings.DownloadDir, filepath.FromSlash(selectedRel))
	selectedDir, e = safeLocalDestination(settings.DownloadDir, selectedDir)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(selectedDir, 0700); e != nil {
		return nil, e
	}
	batch := transfer.NewTransferBatch(path.Base(remotePath))
	queued := []transfer.Activity{}
	var walk func(string, int) error
	walk = func(dir string, depth int) error {
		if depth > 64 {
			return fmt.Errorf("remote directory nesting exceeds 64")
		}
		items, err := client.ListDirectory(a.ctx, dir)
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.IsDir {
				rel := strings.TrimPrefix(strings.TrimPrefix(item.Path, root), "/")
				local := filepath.Join(settings.DownloadDir, filepath.FromSlash(rel))
				local, err = safeLocalDestination(settings.DownloadDir, local)
				if err != nil {
					return err
				}
				if err = os.MkdirAll(local, 0700); err != nil {
					return err
				}
				if err = walk(item.Path, depth+1); err != nil {
					return err
				}
				continue
			}
			rel := strings.TrimPrefix(strings.TrimPrefix(item.Path, root), "/")
			if rel == "" || strings.HasPrefix(rel, "../") {
				return fmt.Errorf("unsafe path returned by server")
			}
			local := filepath.Join(settings.DownloadDir, filepath.FromSlash(rel))
			local, err = safeLocalDestination(settings.DownloadDir, local)
			if err != nil {
				return err
			}
			job, err := a.activity.AddDownload(c.ID, c.Name, item.Path, local, item.Size, settings.DownloadConflict, client, batch)
			if err != nil {
				return err
			}
			queued = append(queued, job)
		}
		return nil
	}
	if e = walk(remotePath, 0); e != nil {
		return queued, e
	}
	return queued, nil
}
func (a *App) QueueUpload(connectionID int64, localPath, remotePath string) (transfer.Activity, error) {
	settings := a.GetSettings()
	c, e := a.connectionByID(connectionID)
	if e != nil {
		return transfer.Activity{}, e
	}
	if e = safeExplorerPath(c.RemoteRoot, remotePath); e != nil {
		return transfer.Activity{}, e
	}
	remotePath = path.Clean("/" + remotePath)
	client, e := a.explorerClient(c)
	if e != nil {
		return transfer.Activity{}, e
	}
	return a.activity.AddUpload(c.ID, c.Name, remotePath, localPath, settings.UploadConflict, client)
}
func (a *App) QueueUploadDirectory(connectionID int64, localDir, remotePath string) ([]transfer.Activity, error) {
	settings := a.GetSettings()
	c, e := a.connectionByID(connectionID)
	if e != nil {
		return nil, e
	}
	if e = safeExplorerPath(c.RemoteRoot, remotePath); e != nil {
		return nil, e
	}
	remotePath = path.Clean("/" + remotePath)
	client, e := a.explorerClient(c)
	if e != nil {
		return nil, e
	}
	base := filepath.Clean(localDir)
	batch := transfer.NewTransferBatch(filepath.Base(base))
	requests := []transfer.UploadRequest{}
	e = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not uploaded")
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(base, p)
		if err != nil {
			return err
		}
		remote := path.Join(remotePath, filepath.ToSlash(rel))
		if err = safeExplorerPath(c.RemoteRoot, remote); err != nil {
			return err
		}
		requests = append(requests, transfer.UploadRequest{RemotePath: remote, LocalPath: p})
		return nil
	})
	if e != nil {
		return nil, e
	}
	return a.activity.AddUploads(c.ID, c.Name, requests, settings.UploadConflict, client, batch)
}

// ClearTransferHistory clears completed/failed/interrupted rows for one connection (0 means all).
func (a *App) ClearTransferHistory(connectionID int64) ([]string, error) {
	return a.activity.ClearHistory(connectionID)
}

func (a *App) GetTransfers() []transfer.Activity { return a.activity.List() }
func (a *App) CancelTransfer(id string) error    { return a.activity.Cancel(id) }
func (a *App) ResolveTransferConflict(id, policy string) error {
	return a.activity.ResolveConflict(id, policy)
}
func (a *App) RemoveTransfer(id string) { a.activity.Remove(id) }
func (a *App) RetryTransfer(id string) error {
	for _, x := range a.activity.List() {
		if x.ID == id {
			if c, err := a.connectionByID(x.ConnectionID); err == nil {
				client, err := a.explorerClient(c)
				if err != nil {
					return err
				}
				return a.activity.Retry(id, client)
			}
			pw, e := a.creds.GetPassword(x.Profile.CredentialKey)
			if e != nil {
				return fmt.Errorf("read connection credential for retry: %w", e)
			}
			client := &transfer.FTPClient{
				Config: transfer.FTPConfig{
					Host:            x.Profile.Host,
					Port:            x.Profile.Port,
					Protocol:        storage.Protocol(x.Profile.Protocol),
					Username:        x.Profile.Username,
					Password:        pw,
					TLSVerify:       x.Profile.TLSVerify,
					CredentialKey:   x.Profile.CredentialKey,
					ReuseConnection: true,
					ForceListHidden: true,
				},
			}
			return a.activity.Retry(id, client)
		}
	}
	return fmt.Errorf("transfer not found")
}
func (a *App) connectionByID(id int64) (*storage.Connection, error) {
	cs, e := a.db.ListConnections()
	if e != nil {
		return nil, e
	}
	for i := range cs {
		if cs[i].ID == id {
			return &cs[i], nil
		}
	}
	return nil, fmt.Errorf("connection not found")
}
func (a *App) ftpClient(c *storage.Connection) (*transfer.FTPClient, error) {
	pw, e := a.creds.GetPassword(c.CredTargetKey)
	if e != nil {
		return nil, fmt.Errorf("read connection credential: %w", e)
	}
	return &transfer.FTPClient{
		Config: transfer.FTPConfig{
			Host:            c.Host,
			Port:            c.Port,
			Protocol:        c.Protocol,
			Username:        c.Username,
			Password:        pw,
			TLSVerify:       c.TLSVerify,
			CredentialKey:   c.CredTargetKey,
			ReuseConnection: true,
			ForceListHidden: true,
		},
	}, nil
}

func (a *App) explorerClient(c *storage.Connection) (*transfer.FTPClient, error) {
	a.explorerMu.Lock()
	defer a.explorerMu.Unlock()

	if a.explorerClients == nil {
		a.explorerClients = make(map[int64]*transfer.FTPClient)
	}
	client, err := a.ftpClient(c)
	if err != nil {
		return nil, err
	}
	if existing := a.explorerClients[c.ID]; existing != nil {
		if existing.Config == client.Config {
			return existing, nil
		}
		_ = existing.Close()
	}
	a.explorerClients[c.ID] = client
	return client, nil
}

type RemoteDeleteItem struct {
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// DeleteRemoteEntry removes a single remote file or directory (recursively if isDir is true).
func (a *App) DeleteRemoteEntry(connectionID int64, remotePath string, isDir bool) error {
	return a.DeleteRemoteEntries(connectionID, []RemoteDeleteItem{{Path: remotePath, IsDir: isDir}})
}

// DeleteRemoteEntries removes multiple remote files or directories (recursively if isDir is true).
func (a *App) DeleteRemoteEntries(connectionID int64, items []RemoteDeleteItem) error {
	if len(items) == 0 {
		return nil
	}
	c, err := a.connectionByID(connectionID)
	if err != nil {
		return err
	}
	root := path.Clean("/" + c.RemoteRoot)
	for _, item := range items {
		clean := path.Clean("/" + item.Path)
		if clean == "/" || clean == "." || clean == root {
			return fmt.Errorf("cannot delete remote root directory %s", clean)
		}
		if err = safeExplorerPath(c.RemoteRoot, clean); err != nil {
			return err
		}
	}
	client, err := a.explorerClient(c)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()

	for _, item := range items {
		clean := path.Clean("/" + item.Path)
		if item.IsDir {
			a.activity.RecordLog("INFO", "Deleting remote directory recursively: "+clean)
			if err := client.DeleteDirectory(ctx, clean); err != nil {
				a.activity.RecordLog("ERROR", "Delete directory "+clean+": "+err.Error())
				return fmt.Errorf("failed to delete folder %s: %w", path.Base(clean), err)
			}
		} else {
			a.activity.RecordLog("INFO", "Deleting remote file: "+clean)
			if err := client.Delete(ctx, clean); err != nil {
				a.activity.RecordLog("ERROR", "Delete file "+clean+": "+err.Error())
				return fmt.Errorf("failed to delete file %s: %w", path.Base(clean), err)
			}
		}
		a.activity.RecordLog("INFO", "Deleted: "+clean)
	}
	return nil
}
func safeExplorerPath(root, p string) error {
	if strings.Contains(p, "\\") || strings.ContainsRune(p, 0) {
		return fmt.Errorf("unsafe remote path")
	}
	clean := path.Clean("/" + p)
	base := path.Clean("/" + root)
	if clean != base && !strings.HasPrefix(clean, strings.TrimRight(base, "/")+"/") {
		return fmt.Errorf("remote path outside configured root")
	}
	for _, s := range strings.Split(p, "/") {
		if s == ".." {
			return fmt.Errorf("remote path traversal rejected")
		}
	}
	return nil
}
func safeLocalDestination(root, dest string) (string, error) {
	base, e := filepath.Abs(root)
	if e != nil {
		return "", e
	}
	target, e := filepath.Abs(dest)
	if e != nil {
		return "", e
	}
	rel, e := filepath.Rel(base, target)
	if e != nil {
		return "", e
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("destination is outside the configured download directory")
	}
	if e = os.MkdirAll(base, 0700); e != nil {
		return "", fmt.Errorf("create download directory: %w", e)
	}
	rootReal, e := filepath.EvalSymlinks(base)
	if e != nil {
		return "", e
	}
	probe := target
	for {
		if _, e = os.Lstat(probe); e == nil {
			resolved, re := filepath.EvalSymlinks(probe)
			if re != nil {
				return "", re
			}
			inside, re := filepath.Rel(rootReal, resolved)
			if re != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(os.PathSeparator)) {
				return "", fmt.Errorf("destination resolves outside the configured download directory")
			}
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", fmt.Errorf("could not resolve destination parent")
		}
		probe = parent
	}
	return target, nil
}

func (a *App) GetProjectDeployments(projectID int64, limit int) ([]DeploymentDTO, error) {
	if limit < 1 || limit > 500 {
		limit = 200
	}
	records, err := a.db.ListDeployments(limit, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]DeploymentDTO, len(records))
	for i, record := range records {
		out[i] = a.deploymentToDTO(record)
	}
	return out, nil
}
func (a *App) GetDeploymentLogs(id int64, offset, limit int) (storage.DeploymentLogPage, error) {
	return a.db.DeploymentLogs(id, offset, limit)
}
func (a *App) ClearDeploymentLogs(id int64) error { return a.db.ClearDeploymentLogs(id) }

// GetDeploymentHistory returns the most recent deployments.
func (a *App) GetDeploymentHistory(limit int) ([]DeploymentDTO, error) {
	if limit <= 0 {
		limit = 50
	}
	deps, err := a.db.ListDeployments(limit)
	if err != nil {
		return nil, err
	}
	out := make([]DeploymentDTO, len(deps))
	for i, d := range deps {
		out[i] = a.deploymentToDTO(d)
	}
	return out, nil
}

// GetDeployment returns a single deployment by ID with its resolved configuration metadata.
func (a *App) GetDeployment(id int64) (*DeploymentDTO, error) {
	d, err := a.db.GetDeployment(id)
	if err != nil {
		return nil, err
	}
	dto := a.deploymentToDTO(*d)
	return &dto, nil
}

// GetPendingDeployments returns all currently running deployments.
func (a *App) GetPendingDeployments() ([]DeploymentDTO, error) {
	deps, err := a.db.PendingDeployments()
	if err != nil {
		return nil, err
	}
	out := make([]DeploymentDTO, len(deps))
	for i, d := range deps {
		out[i] = a.deploymentToDTO(d)
	}
	return out, nil
}

// GetDeploymentFiles returns journaled file entries for a deployment and stage.
func (a *App) GetDeploymentFiles(deploymentID int64, stage string) ([]DeploymentFileDTO, error) {
	files, err := a.db.GetDeploymentFiles(deploymentID, stage)
	if err != nil {
		return nil, err
	}
	out := make([]DeploymentFileDTO, len(files))
	for i, f := range files {
		out[i] = deploymentFileToDTO(f)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// DTO converters — handle sql.NullString → string, sql.NullTime → ISO string.
// ---------------------------------------------------------------------------

func connectionToDTO(c storage.Connection) ConnectionDTO {
	return ConnectionDTO{
		ID:         c.ID,
		Name:       c.Name,
		Host:       c.Host,
		Port:       c.Port,
		Protocol:   string(c.Protocol),
		Username:   c.Username,
		RemoteRoot: c.RemoteRoot,
		TLSVerify:  c.TLSVerify,
		CreatedAt:  c.CreatedAt.Format(time.RFC3339),
	}
}

func projectToDTO(p storage.Project) ProjectDTO {
	return ProjectDTO{
		ID:                p.ID,
		Name:              p.Name,
		ProjectType:       p.Type,
		LocalPath:         p.LocalPath,
		BuildCommand:      p.BuildCommand,
		OutputRelPath:     p.OutputRelPath,
		ArtifactRulesJSON: p.ArtifactRulesJSON,
	}
}

func (a *App) deploymentToDTO(d storage.Deployment) DeploymentDTO {
	errMsg := ""
	if d.ErrorMessage.Valid {
		errMsg = d.ErrorMessage.String
	}
	finished := ""
	if d.FinishedAt.Valid {
		finished = d.FinishedAt.Time.Format(time.RFC3339)
	}
	dto := DeploymentDTO{
		ID:           d.ID,
		ProjectID:    d.ProjectID,
		ConnectionID: d.ConnectionID,
		Status:       d.Status,
		CurrentPhase: d.CurrentPhase,
		BackupDir:    d.BackupDir,
		ErrorMessage: errMsg,
		StartedAt:    d.StartedAt.Format(time.RFC3339),
		FinishedAt:   finished,
		Cancelled:    strings.Contains(errMsg, context.Canceled.Error()),
	}

	if a.db != nil {
		var p storage.Project
		if err := a.db.Conn.QueryRow(`SELECT name, project_type, build_cmd, output_rel_path FROM projects WHERE id=?`, d.ProjectID).Scan(&p.Name, &p.Type, &p.BuildCommand, &p.OutputRelPath); err == nil {
			dto.ProjectName = p.Name
			dto.ProjectType = p.Type
			dto.BuildCommand = p.BuildCommand
			dto.OutputRelPath = p.OutputRelPath
		}
		var c storage.Connection
		if err := a.db.Conn.QueryRow(`SELECT name, host, remote_root FROM connections WHERE id=?`, d.ConnectionID).Scan(&c.Name, &c.Host, &c.RemoteRoot); err == nil {
			dto.ConnectionName = c.Name
			dto.ConnectionHost = c.Host
			dto.RemoteRoot = c.RemoteRoot
		}
	}

	return dto
}

func deploymentFileToDTO(f storage.DeploymentFile) DeploymentFileDTO {
	sha := ""
	if f.SHA256Local.Valid {
		sha = f.SHA256Local.String
	}
	errDetail := ""
	if f.ErrorDetail.Valid {
		errDetail = f.ErrorDetail.String
	}
	return DeploymentFileDTO{
		ID:           f.ID,
		DeploymentID: f.DeploymentID,
		RelativePath: f.RelativePath,
		Stage:        f.Stage,
		Status:       f.Status,
		SizeBytes:    f.SizeBytes,
		SHA256Local:  sha,
		ErrorDetail:  errDetail,
		UpdatedAt:    f.UpdatedAt.Format(time.RFC3339),
	}
}

// Ensure transfer package is importable (FTPConfig.Protocol uses storage.Protocol).
var _ = transfer.FTPConfig{}
