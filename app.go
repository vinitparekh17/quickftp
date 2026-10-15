package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jlaffaye/ftp"
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
	ID           int64  `json:"id"`
	ProjectID    int64  `json:"projectId"`
	ConnectionID int64  `json:"connectionId"`
	Status       string `json:"status"`
	CurrentPhase string `json:"currentPhase"`
	BackupDir    string `json:"backupDir"`
	ErrorMessage string `json:"errorMessage"`
	StartedAt    string `json:"startedAt"`
	FinishedAt   string `json:"finishedAt"`
	Cancelled    bool   `json:"cancelled"`
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
	BackupDir string `json:"backupDir"`
}

// ---------------------------------------------------------------------------
// App struct — Wails binds every exported method.
// ---------------------------------------------------------------------------

// App holds the application state shared with the Wails runtime.
type App struct {
	ctx          context.Context
	db           *storage.DB
	creds        creds.Store
	service      *deploy.Service
	deployMu     sync.Mutex
	deployCancel context.CancelFunc
	settingsPath string
}

// startup is called by Wails after the frontend is ready.
func (a *App) startup(ctx context.Context) {
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
	if raw, readErr := os.ReadFile(settingsPath); readErr == nil {
		var settings SettingsDTO
		if json.Unmarshal(raw, &settings) == nil && settings.BackupDir != "" {
			if abs, absErr := filepath.Abs(settings.BackupDir); absErr == nil {
				backupRoot = abs
			}
		}
	}
	a.settingsPath = settingsPath
	a.service = &deploy.Service{
		DB:          db,
		Credentials: a.creds,
		BackupRoot:  backupRoot,
	}
}

func (a *App) GetSettings() SettingsDTO { return SettingsDTO{BackupDir: a.service.BackupRoot} }

func (a *App) SelectDirectory(title string) (string, error) {
	if title == "" {
		title = "Select a directory"
	}
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{Title: title})
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
	if a.deployCancel != nil {
		return fmt.Errorf("settings cannot be changed during a deployment")
	}
	if err = os.MkdirAll(backupDir, 0700); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	data, err := json.MarshalIndent(SettingsDTO{BackupDir: backupDir}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(a.settingsPath, data, 0600); err != nil {
		return err
	}
	a.service.BackupRoot = backupDir
	return nil
}

// shutdown is called by Wails when the application is closing.
func (a *App) shutdown(_ context.Context) {
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
	if err := a.db.UpsertConnection(c); err != nil {
		return fmt.Errorf("save connection: %w", err)
	}
	if dto.Password != "" {
		if err := a.creds.SavePassword(targetKey, dto.Username, dto.Password); err != nil {
			return fmt.Errorf("save credential: %w", err)
		}
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
	if targetKey != "" {
		_ = a.creds.DeletePassword(targetKey) // best-effort
	}
	return nil
}

// TestConnection attempts to dial, authenticate, and disconnect.
func (a *App) TestConnection(dto ConnectionDTO) error {
	port := dto.Port
	if port == 0 {
		port = 21
	}
	addr := fmt.Sprintf("%s:%d", dto.Host, port)
	opts := []ftp.DialOption{
		ftp.DialWithTimeout(15 * time.Second),
	}

	protocol := storage.Protocol(dto.Protocol)
	if protocol == storage.ProtocolFTPSExplicit {
		opts = append(opts, ftp.DialWithExplicitTLS(&tls.Config{
			ServerName:         dto.Host,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: !dto.TLSVerify,
		}))
	}

	conn, err := ftp.Dial(addr, opts...)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	if err := conn.Login(dto.Username, dto.Password); err != nil {
		_ = conn.Quit()
		return fmt.Errorf("login: %w", err)
	}
	return conn.Quit()
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
	if a.deployCancel != nil {
		a.deployMu.Unlock()
		return fmt.Errorf("a deployment is already running")
	}
	deployCtx, cancel := context.WithCancel(a.ctx)
	a.deployCancel = cancel
	a.deployMu.Unlock()
	// Wire the build-log callback to emit events to the frontend.
	a.service.BuildLog = func(stream, line string) {
		wailsRuntime.EventsEmit(a.ctx, "deploy:log", map[string]string{
			"stream": stream,
			"line":   line,
		})
	}
	a.service.PhaseLog = func(phase string) {
		wailsRuntime.EventsEmit(a.ctx, "deploy:phase", map[string]interface{}{"phase": phase})
	}
	a.service.TransferLog = func(stream, line string) {
		wailsRuntime.EventsEmit(a.ctx, "deploy:log", map[string]string{"stream": stream, "line": line})
	}

	deployOpts := deploy.Options{
		IISOfflinePage:   opts.IISOfflinePage,
		IISShutdownDelay: time.Duration(opts.IISShutdownDelay) * time.Millisecond,
	}

	go func() {
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

		deploymentID, err := a.service.DeployProject(deployCtx, projectName, connectionName, deployOpts)
		if err != nil {
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
	return a.service.Rollback(a.ctx, deploymentID, connectionName)
}

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
		out[i] = deploymentToDTO(d)
	}
	return out, nil
}

// GetPendingDeployments returns all currently running deployments.
func (a *App) GetPendingDeployments() ([]DeploymentDTO, error) {
	deps, err := a.db.PendingDeployments()
	if err != nil {
		return nil, err
	}
	out := make([]DeploymentDTO, len(deps))
	for i, d := range deps {
		out[i] = deploymentToDTO(d)
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

func deploymentToDTO(d storage.Deployment) DeploymentDTO {
	errMsg := ""
	if d.ErrorMessage.Valid {
		errMsg = d.ErrorMessage.String
	}
	finished := ""
	if d.FinishedAt.Valid {
		finished = d.FinishedAt.Time.Format(time.RFC3339)
	}
	return DeploymentDTO{
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
