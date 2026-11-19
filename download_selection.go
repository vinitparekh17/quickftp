package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"quickftp/internal/transfer"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type DownloadSelectionItem struct {
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

type DownloadSelectionResult struct {
	Queued int      `json:"queued"`
	Errors []string `json:"errors"`
}

// DownloadSelection prompts once for the destination of the entire selection.
func (a *App) DownloadSelection(connectionID int64, items []DownloadSelectionItem) (*DownloadSelectionResult, error) {
	if len(items) == 0 {
		return nil, nil
	}
	c, err := a.connectionByID(connectionID)
	if err != nil {
		return nil, err
	}
	settings := a.GetSettings()
	options := wailsRuntime.OpenDialogOptions{Title: "Choose download folder", CanCreateDirectories: true}
	if info, err := os.Stat(settings.DownloadDir); err == nil && info.IsDir() {
		options.DefaultDirectory = settings.DownloadDir
	}
	destination, err := wailsRuntime.OpenDirectoryDialog(a.ctx, options)
	if err != nil || destination == "" {
		return nil, err
	}
	client, err := a.explorerClient(c)
	if err != nil {
		return nil, err
	}

	result := &DownloadSelectionResult{}
	for _, item := range items {
		if err := safeExplorerPath(c.RemoteRoot, item.Path); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", item.Path, err))
			continue
		}
		remote := path.Clean("/" + item.Path)
		parent := path.Dir(remote)
		if item.IsDir {
			jobs, err := a.queueDownloadTree(c, client, destination, parent, remote, settings.DownloadConflict)
			result.Queued += len(jobs)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", item.Path, err))
			}
			continue
		}
		local, err := downloadDestination(destination, parent, remote)
		if err == nil {
			_, err = a.activity.AddDownloadWithin(c.ID, c.Name, remote, destination, local, item.Size, settings.DownloadConflict, client)
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", item.Path, err))
		} else {
			result.Queued++
		}
	}
	return result, nil
}

func downloadDestination(destination, remoteBase, remote string) (string, error) {
	if err := safeExplorerPath(remoteBase, remote); err != nil {
		return "", err
	}
	clean := path.Clean("/" + remote)
	if clean == remoteBase {
		return transfer.ValidateDownloadDestination(destination, destination)
	}
	rel := filepath.FromSlash(strings.TrimPrefix(clean, strings.TrimRight(remoteBase, "/")+"/"))
	if !filepath.IsLocal(rel) {
		return "", fmt.Errorf("invalid local download path")
	}
	return transfer.ValidateDownloadDestination(destination, filepath.Join(destination, rel))
}
