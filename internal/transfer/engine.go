// Package transfer implements journaled, restart-aware FTP deployments.
package transfer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"quickftp/internal/storage"
)

var ErrNotFound = errors.New("remote file not found")

type RemoteFile struct {
	Path      string     `json:"path"`
	Size      int64      `json:"size"`
	SizeKnown bool       `json:"sizeKnown"`
	IsDir     bool       `json:"isDir"`
	Modified  *time.Time `json:"modified,omitempty"`
}

// Client methods return errors when the server's outcome cannot be confirmed.
// The engine reconciles uncertain writes/deletes by inspecting remote state.
type Client interface {
	List(context.Context, string) ([]RemoteFile, error)
	Download(context.Context, string, io.Writer) error
	Upload(context.Context, string, io.Reader) error
	Rename(context.Context, string, string) error
	Delete(context.Context, string) error
}

type Artifact struct{ LocalPath, RelativePath string }
type Engine struct {
	DB                     *storage.DB
	Client                 Client
	RemoteRoot, BackupRoot string
	OnPhase                func(string)
	OnLog                  func(string, string)
	// A non-empty page opts into the ASP.NET Core on IIS offline-file lifecycle.
	IISOfflinePage   string
	IISShutdownDelay time.Duration
	Gate             *Gate
	ConnectionID     int64
	ConnectionName   string
	OnFileTransfer   func(deploymentID int64, connectionID int64, connectionName, remote, local string, total, bytes int64, status, detail string)
}

func (e *Engine) Deploy(ctx context.Context, projectID, connectionID int64, artifacts []Artifact) (deploymentID int64, retErr error) {
	if e.DB == nil || e.Client == nil {
		return 0, errors.New("deployment engine requires database and transfer client")
	}
	if len(artifacts) == 0 {
		return 0, errors.New("no deployment artifacts selected")
	}
	if err := os.MkdirAll(e.BackupRoot, 0700); err != nil {
		return 0, fmt.Errorf("create backup directory: %w", err)
	}
	id, err := e.DB.StartDeployment(projectID, connectionID, e.BackupRoot)
	if err != nil {
		return 0, err
	}
	backupDir := filepath.Join(e.BackupRoot, fmt.Sprintf("deployment-%d", id))
	if err = os.MkdirAll(backupDir, 0700); err != nil {
		_ = e.DB.FinishDeployment(id, "failed", err.Error())
		return id, fmt.Errorf("create deployment backup directory: %w", err)
	}
	if err = e.DB.SetDeploymentBackupDir(id, backupDir); err != nil {
		_ = e.DB.FinishDeployment(id, "failed", err.Error())
		return id, err
	}
	markerPath := remoteJoin(e.RemoteRoot, "app_offline.htm")
	markerArmed := false
	defer func() {
		if markerArmed {
			cleanupErr := e.Client.Delete(context.Background(), markerPath)
			if cleanupErr != nil && !errors.Is(cleanupErr, ErrNotFound) {
				if retErr == nil {
					retErr = fmt.Errorf("remove IIS offline marker: %w", cleanupErr)
				}
				_ = e.DB.FinishDeployment(id, "failed", retErr.Error())
			}
		}
	}()
	fail := func(phase string, cause error) (int64, error) {
		e.setPhase(id, phase)
		// Keep cancelled runs in the recoverable failed state so rollback remains available.
		_ = e.DB.FinishDeployment(id, "failed", cause.Error())
		return id, cause
	}
	seen := make(map[string]bool, len(artifacts))
	files := make([]prepared, len(artifacts))
	for i, a := range artifacts {
		rel, err := safeRelative(a.RelativePath)
		if err != nil {
			return fail("planning", err)
		}
		if seen[rel] {
			return fail("planning", fmt.Errorf("duplicate artifact path %q", rel))
		}
		seen[rel] = true
		info, err := os.Stat(a.LocalPath)
		if err != nil {
			return fail("planning", fmt.Errorf("stat artifact %s: %w", rel, err))
		}
		if !info.Mode().IsRegular() {
			return fail("planning", fmt.Errorf("artifact %s is not a regular file", rel))
		}
		digest, size, err := hashFile(a.LocalPath)
		if err != nil {
			return fail("planning", err)
		}
		files[i] = prepared{Artifact: a, relative: rel, digest: digest, size: size}
	}
	// Snapshot every currently deployed file before touching the remote tree.
	e.setPhase(id, "discover")
	remote, err := e.Client.List(ctx, e.RemoteRoot)
	if err != nil {
		return fail("backup", fmt.Errorf("list remote root: %w", err))
	}
	backed := map[string]bool{}
	e.setPhase(id, "backup")
	backupTotal := 0
	for _, rf := range remote {
		if !rf.IsDir {
			backupTotal++
		}
	}
	backupDone := 0
	for _, rf := range remote {
		if err := ctx.Err(); err != nil {
			return fail("backup", err)
		}
		if rf.IsDir {
			continue
		}
		backupDone++
		rel, err := remoteRelative(e.RemoteRoot, rf.Path)
		if err != nil {
			return fail("backup", err)
		}
		dst, err := localJoin(backupDir, rel)
		if err != nil {
			return fail("backup", err)
		}
		if err = os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
			return fail("backup", err)
		}
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err != nil {
			return fail("backup", err)
		}
		err = e.Client.Download(ctx, rf.Path, f)
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			_ = e.DB.JournalFile(id, rel, "backup", "failed", rf.Size, "", err.Error())
			return fail("backup", fmt.Errorf("backup %s: %w", rel, err))
		}
		e.setPhase(id, "verify_backup")
		backupHash, backupSize, hashErr := hashFile(dst)
		if hashErr != nil || backupSize != rf.Size {
			if hashErr == nil {
				hashErr = fmt.Errorf("backup size mismatch: expected %d bytes, got %d", rf.Size, backupSize)
			}
			return fail("backup", fmt.Errorf("verify backup %s: %w", rel, hashErr))
		}
		if err = e.DB.JournalFile(id, rel, "backup", "verified", backupSize, backupHash, ""); err != nil {
			return fail("backup", err)
		}
		e.log("stdout", fmt.Sprintf("Backup verified %s (%d/%d, %d bytes)", rel, backupDone, backupTotal, backupSize))
		backed[rel] = true
	}
	if err := ctx.Err(); err != nil {
		return fail("backup", err)
	}
	if e.IISOfflinePage != "" {
		if backed["app_offline.htm"] {
			return fail("backup", errors.New("remote app_offline.htm already exists; refusing to replace an existing offline marker"))
		}
		markerArmed = true // Also clean up when STOR succeeds but its final reply is lost.
		markerHash := sha256.Sum256([]byte(e.IISOfflinePage))
		markerDigest := hex.EncodeToString(markerHash[:])
		if err := e.DB.JournalFile(id, "app_offline.htm", "upload", "pending", int64(len(e.IISOfflinePage)), markerDigest, ""); err != nil {
			return fail("iis_offline", err)
		}
		release := func() {}
		if e.Gate != nil {
			var gateErr error
			release, gateErr = e.Gate.Acquire(ctx)
			if gateErr != nil {
				return fail("iis_offline", gateErr)
			}
		}
		uploadErr := e.Client.Upload(ctx, markerPath, bytes.NewBufferString(e.IISOfflinePage))
		release()
		if uploadErr != nil {
			_ = e.DB.JournalFile(id, "app_offline.htm", "upload", "failed", int64(len(e.IISOfflinePage)), markerDigest, uploadErr.Error())
			return fail("iis_offline", fmt.Errorf("activate IIS offline marker: %w", uploadErr))
		}
		if err := e.DB.JournalFile(id, "app_offline.htm", "upload", "completed", int64(len(e.IISOfflinePage)), markerDigest, ""); err != nil {
			return fail("iis_offline", err)
		}
		_ = e.DB.SetDeploymentPhase(id, "iis_offline")
		delay := e.IISShutdownDelay
		if delay <= 0 {
			delay = 1500 * time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fail("iis_offline", ctx.Err())
		case <-timer.C:
		}
	}
	// Upload each artifact to a unique staging name, then atomically rename it.
	e.setPhase(id, "upload")
	uploadBytes := int64(0)
	for i, f := range files {
		if err := ctx.Err(); err != nil {
			return fail("upload", err)
		}
		target := remoteJoin(e.RemoteRoot, f.relative)
		tmp := target + fmt.Sprintf(".quickftp-%d.tmp", id)
		if err := e.DB.JournalFile(id, f.relative, "upload", "pending", f.size, f.digest, ""); err != nil {
			return fail("upload", err)
		}
		e.log("stdout", fmt.Sprintf("Uploading %s (%d/%d, %d bytes)", f.relative, i+1, len(files), f.size))
		e.fileTransfer(id, target, f.LocalPath, f.size, 0, "Queued", "")
		src, err := os.Open(f.LocalPath)
		if err != nil {
			e.fileTransfer(id, target, f.LocalPath, f.size, 0, "Failed", err.Error())
			return fail("upload", err)
		}
		release := func() {}
		if e.Gate != nil {
			var gateErr error
			release, gateErr = e.Gate.Acquire(ctx)
			if gateErr != nil {
				_ = src.Close()
				e.fileTransfer(id, target, f.LocalPath, f.size, 0, "Failed", gateErr.Error())
				return fail("upload", gateErr)
			}
		}
		e.fileTransfer(id, target, f.LocalPath, f.size, 0, "Running", "")
		err = e.Client.Upload(ctx, tmp, &engineProgressReader{r: src, onBytes: func(n int64) { e.fileTransfer(id, target, f.LocalPath, f.size, n, "Running", "") }})
		release()
		_ = src.Close()
		if err != nil { // A lost reply may follow a successful STOR; compare the staged bytes before deciding.
			ok, checkErr := e.remoteMatches(ctx, tmp, f.digest)
			if checkErr != nil || !ok {
				e.fileTransfer(id, target, f.LocalPath, f.size, 0, "Failed", err.Error())
				_ = e.DB.JournalFile(id, f.relative, "upload", "failed", f.size, f.digest, err.Error())
				return fail("upload", fmt.Errorf("upload %s outcome unresolved: %w", f.relative, err))
			}
		}
		ok, err := e.remoteMatches(ctx, tmp, f.digest)
		if err != nil || !ok {
			if err == nil {
				err = errors.New("staged upload checksum mismatch")
			}
			e.fileTransfer(id, target, f.LocalPath, f.size, 0, "Failed", err.Error())
			return fail("upload", fmt.Errorf("verify %s: %w", f.relative, err))
		}
		if err := ctx.Err(); err != nil {
			e.fileTransfer(id, target, f.LocalPath, f.size, 0, "Failed", err.Error())
			return fail("upload", err)
		}
		if backed[f.relative] {
			// The previous version is safely backed up and the staged version verified.
			// Remove the old target before promotion because RNTO replacement is not
			// consistently supported by FTP servers.
			if err = e.Client.Delete(ctx, target); err != nil {
				listing, listErr := e.Client.List(ctx, path.Dir(target))
				stillThere := false
				if listErr == nil {
					for _, item := range listing {
						if item.Path == target {
							stillThere = true
						}
					}
				}
				if listErr != nil || stillThere {
					e.fileTransfer(id, target, f.LocalPath, f.size, 0, "Failed", err.Error())
					return fail("upload", fmt.Errorf("replace %s outcome unresolved: %w", f.relative, err))
				}
			}
			if err := ctx.Err(); err != nil {
				e.fileTransfer(id, target, f.LocalPath, f.size, 0, "Failed", err.Error())
				return fail("upload", err)
			}
		}
		if err = e.Client.Rename(ctx, tmp, target); err != nil { // Reconcile rename outcome before recording failure.
			ok, checkErr := e.remoteMatches(ctx, target, f.digest)
			if checkErr != nil || !ok {
				e.fileTransfer(id, target, f.LocalPath, f.size, 0, "Failed", err.Error())
				return fail("upload", fmt.Errorf("promote %s outcome unresolved: %w", f.relative, err))
			}
		}
		e.setPhase(id, "verify_upload")
		ok, err = e.remoteMatches(ctx, target, f.digest)
		if err != nil || !ok {
			if err == nil {
				err = errors.New("remote checksum mismatch")
			}
			e.fileTransfer(id, target, f.LocalPath, f.size, 0, "Failed", err.Error())
			return fail("verify", fmt.Errorf("verify %s: %w", f.relative, err))
		}
		if err = e.DB.JournalFile(id, f.relative, "upload", "verified", f.size, f.digest, ""); err != nil {
			e.fileTransfer(id, target, f.LocalPath, f.size, 0, "Failed", err.Error())
			return fail("upload", err)
		}
		uploadBytes += f.size
		e.log("success", fmt.Sprintf("Upload verified %s (%d/%d, %d/%d bytes)", f.relative, i+1, len(files), uploadBytes, totalArtifactBytes(files)))
		e.fileTransfer(id, target, f.LocalPath, f.size, f.size, "Completed", "")
	}
	// Delete stale files only after all new artifacts are backed up and verified.
	e.setPhase(id, "delete")
	for _, rf := range remote {
		if err := ctx.Err(); err != nil {
			return fail("delete", err)
		}
		if rf.IsDir {
			continue
		}
		rel, err := remoteRelative(e.RemoteRoot, rf.Path)
		if err != nil {
			return fail("delete", err)
		}
		if seen[rel] {
			continue
		}
		if !backed[rel] {
			return fail("delete", fmt.Errorf("refusing to delete %s without a verified backup", rel))
		}
		e.log("stdout", "Removing stale remote file "+rel)
		if err = e.Client.Delete(ctx, rf.Path); err != nil { // If the file is gone, the command likely succeeded; otherwise preserve the failure for recovery.
			listing, listErr := e.Client.List(ctx, path.Dir(rf.Path))
			exists := false
			if listErr == nil {
				for _, item := range listing {
					if item.Path == rf.Path {
						exists = true
					}
				}
			}
			if listErr != nil || exists {
				_ = e.DB.JournalFile(id, rel, "delete", "failed", rf.Size, "", err.Error())
				return fail("delete", fmt.Errorf("delete %s outcome unresolved: %w", rel, err))
			}
		}
		if err = e.DB.JournalFile(id, rel, "delete", "completed", rf.Size, "", ""); err != nil {
			return fail("delete", err)
		}
	}
	e.setPhase(id, "complete")
	if err = e.DB.FinishDeployment(id, "completed", ""); err != nil {
		return id, err
	}
	return id, nil
}

func (e *Engine) fileTransfer(id int64, remote, local string, total, bytes int64, status, detail string) {
	if e.OnFileTransfer != nil {
		e.OnFileTransfer(id, e.ConnectionID, e.ConnectionName, remote, local, total, bytes, status, detail)
	}
}

type engineProgressReader struct {
	r       io.Reader
	onBytes func(int64)
	n, last int64
}

func (r *engineProgressReader) Read(p []byte) (int, error) {
	n, e := r.r.Read(p)
	r.n += int64(n)
	if r.n-r.last >= 512*1024 {
		r.last = r.n
		r.onBytes(r.n)
	}
	return n, e
}

func (e *Engine) setPhase(id int64, phase string) {
	_ = e.DB.SetDeploymentPhase(id, phase)
	if e.OnPhase != nil {
		e.OnPhase(phase)
	}
}

func (e *Engine) log(stream, line string) {
	if e.OnLog != nil {
		e.OnLog(stream, line)
	}
}
func totalArtifactBytes(files []prepared) int64 {
	var total int64
	for _, f := range files {
		total += f.size
	}
	return total
}

// Rollback restores the last verified remote snapshot for a failed or interrupted deployment.
// It intentionally requires an explicit deployment ID so recovery never guesses which run to undo.
func (e *Engine) Rollback(ctx context.Context, deploymentID int64) error {
	d, err := e.DB.GetDeployment(deploymentID)
	if err != nil {
		return err
	}
	if d.Status != "failed" && d.Status != "running" {
		return fmt.Errorf("deployment %d is %s and cannot be rolled back", deploymentID, d.Status)
	}
	backups, err := e.DB.GetDeploymentFiles(deploymentID, "backup")
	if err != nil {
		return err
	}
	backed := make(map[string]bool)
	for _, f := range backups {
		if f.Status != "verified" {
			continue
		}
		rel, err := safeRelative(f.RelativePath)
		if err != nil {
			return err
		}
		backed[rel] = true
		local, err := localJoin(d.BackupDir, rel)
		if err != nil {
			return err
		}
		digest, size, err := hashFile(local)
		if err != nil {
			return fmt.Errorf("read backup %s: %w", rel, err)
		}
		if f.SHA256Local.Valid && digest != f.SHA256Local.String {
			return fmt.Errorf("backup %s checksum changed", rel)
		}
		if f.SizeBytes != size {
			return fmt.Errorf("backup %s size changed", rel)
		}
		target := remoteJoin(e.RemoteRoot, rel)
		tmp := target + fmt.Sprintf(".quickftp-rollback-%d.tmp", deploymentID)
		src, err := os.Open(local)
		if err != nil {
			return err
		}
		release := func() {}
		if e.Gate != nil {
			var gateErr error
			release, gateErr = e.Gate.Acquire(ctx)
			if gateErr != nil {
				_ = src.Close()
				return gateErr
			}
		}
		err = e.Client.Upload(ctx, tmp, src)
		release()
		_ = src.Close()
		if err != nil {
			ok, verifyErr := e.remoteMatches(ctx, tmp, digest)
			if verifyErr != nil || !ok {
				return fmt.Errorf("restore upload %s unresolved: %w", rel, err)
			}
		}
		ok, err := e.remoteMatches(ctx, tmp, digest)
		if err != nil || !ok {
			if err == nil {
				err = errors.New("restored staging checksum mismatch")
			}
			return fmt.Errorf("verify restore %s: %w", rel, err)
		}
		if err = e.Client.Delete(ctx, target); err != nil {
			listing, listErr := e.Client.List(ctx, path.Dir(target))
			exists := false
			if listErr == nil {
				for _, item := range listing {
					if item.Path == target {
						exists = true
					}
				}
			}
			if listErr != nil || exists {
				return fmt.Errorf("remove changed file %s: %w", rel, err)
			}
		}
		if err = e.Client.Rename(ctx, tmp, target); err != nil {
			ok, verifyErr := e.remoteMatches(ctx, target, digest)
			if verifyErr != nil || !ok {
				return fmt.Errorf("restore promotion %s unresolved: %w", rel, err)
			}
		}
	}
	// Remove artifacts introduced by this run, plus known staging files left by a crash.
	uploads, err := e.DB.GetDeploymentFiles(deploymentID, "upload")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, f := range uploads {
		rel, err := safeRelative(f.RelativePath)
		if err != nil {
			return err
		}
		if seen[rel] {
			continue
		}
		seen[rel] = true
		if !backed[rel] {
			target := remoteJoin(e.RemoteRoot, rel)
			if err := e.Client.Delete(ctx, target); err != nil {
				listing, listErr := e.Client.List(ctx, path.Dir(target))
				exists := false
				if listErr == nil {
					for _, item := range listing {
						if item.Path == target {
							exists = true
						}
					}
				}
				if listErr != nil || exists {
					return fmt.Errorf("remove new artifact %s: %w", rel, err)
				}
			}
		}
		tmp := remoteJoin(e.RemoteRoot, rel) + fmt.Sprintf(".quickftp-%d.tmp", deploymentID)
		_ = e.Client.Delete(ctx, tmp)
	}
	return e.DB.FinishDeployment(deploymentID, "rolled_back", "")
}

type prepared struct {
	Artifact
	relative, digest string
	size             int64
}

func (e *Engine) remoteMatches(ctx context.Context, remote, want string) (bool, error) {
	r, w := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- e.Client.Download(ctx, remote, w); _ = w.Close() }()
	h := sha256.New()
	_, copyErr := io.Copy(h, r)
	downloadErr := <-done
	if copyErr != nil {
		return false, copyErr
	}
	if downloadErr != nil {
		return false, downloadErr
	}
	return hex.EncodeToString(h.Sum(nil)) == want, nil
}
func hashFile(name string) (string, int64, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}
func safeRelative(s string) (string, error) {
	s = strings.ReplaceAll(s, "\\", "/")
	clean := path.Clean(s)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(clean, ":") {
		return "", fmt.Errorf("unsafe relative path %q", s)
	}
	return clean, nil
}
func remoteJoin(root, rel string) string { return path.Join(root, rel) }
func remoteRelative(root, p string) (string, error) {
	root = path.Clean(root)
	p = path.Clean(p)
	if p == root {
		return "", fmt.Errorf("remote path %q is the root directory", p)
	}
	prefix := strings.TrimSuffix(root, "/") + "/"
	if !strings.HasPrefix(p, prefix) {
		return "", fmt.Errorf("remote path %q is outside root %q", p, root)
	}
	return safeRelative(strings.TrimPrefix(p, prefix))
}
func localJoin(root, rel string) (string, error) {
	rel, err := safeRelative(rel)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(rel)), nil
}
