package transfer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"quickftp/internal/storage"
)

// Run against the local Docker server with QUICKFTP_FTP_INTEGRATION=1 and the
// QUICKFTP_FTP_* connection variables used by cmd/ftp-spike. This exercises
// the same shared-client ActivityManager flow used for directory uploads.
func TestFTPDirectoryActivityIntegration(t *testing.T) {
	if os.Getenv("QUICKFTP_FTP_INTEGRATION") != "1" {
		t.Skip("set QUICKFTP_FTP_INTEGRATION=1 to run against a live FTP server")
	}
	port, err := strconv.Atoi(os.Getenv("QUICKFTP_FTP_PORT"))
	if err != nil || port < 1 {
		t.Fatal("QUICKFTP_FTP_PORT must be set")
	}
	protocol := storage.Protocol(os.Getenv("QUICKFTP_FTP_PROTOCOL"))
	if protocol == "" {
		protocol = storage.ProtocolFTPSExplicit
	}
	client := &FTPClient{Config: FTPConfig{
		Host: os.Getenv("QUICKFTP_FTP_HOST"), Port: port, Protocol: protocol,
		Username: os.Getenv("QUICKFTP_FTP_USERNAME"), Password: os.Getenv("QUICKFTP_FTP_PASSWORD"),
		TLSVerify: os.Getenv("QUICKFTP_FTP_TLS_VERIFY") != "false", ReuseConnection: true,
	}}
	if client.Config.Host == "" || client.Config.Username == "" {
		t.Fatal("QUICKFTP_FTP_HOST and QUICKFTP_FTP_USERNAME must be set")
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	local := t.TempDir()
	files := 80
	if raw := os.Getenv("QUICKFTP_FTP_INTEGRATION_FILES"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			files = n
		}
	}
	for i := 0; i < files; i++ {
		rel := filepath.Join(".git", "objects", fmt.Sprintf("%02x", i%16), fmt.Sprintf("%040x", i))
		full := filepath.Join(local, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		data := make([]byte, i*37+1)
		for j := range data {
			data[j] = byte(i + j)
		}
		if err := os.WriteFile(full, data, 0600); err != nil {
			t.Fatal(err)
		}
	}

	remoteRoot := path.Join("/site/wwwroot", fmt.Sprintf("quickftp-integration-%d", time.Now().UnixNano()))
	remoteFiles := make([]string, 0, files)
	for i := 0; i < files; i++ {
		rel := filepath.Join(".git", "objects", fmt.Sprintf("%02x", i%16), fmt.Sprintf("%040x", i))
		remoteFiles = append(remoteFiles, path.Join(remoteRoot, filepath.ToSlash(rel)))
	}
	cleanupRemote := func() {
		for _, remote := range remoteFiles {
			_ = client.Delete(context.Background(), remote)
		}
		for i := 0; i < 16; i++ {
			_ = client.DeleteDirectory(context.Background(), path.Join(remoteRoot, ".git", "objects", fmt.Sprintf("%02x", i)))
		}
		_ = client.DeleteDirectory(context.Background(), path.Join(remoteRoot, ".git", "objects"))
		_ = client.DeleteDirectory(context.Background(), path.Join(remoteRoot, ".git"))
		_ = client.DeleteDirectory(context.Background(), remoteRoot)
	}
	cleanupSafe := false
	defer func() {
		if cleanupSafe {
			cleanupRemote()
		}
	}()
	manager := OpenActivityManager(filepath.Join(t.TempDir(), "activity.json"), NewGate(2))
	requests := make([]UploadRequest, 0, files)
	for i := 0; i < files; i++ {
		rel := filepath.Join(".git", "objects", fmt.Sprintf("%02x", i%16), fmt.Sprintf("%040x", i))
		requests = append(requests, UploadRequest{RemotePath: path.Join(remoteRoot, filepath.ToSlash(rel)), LocalPath: filepath.Join(local, rel)})
	}
	if _, err := manager.AddUploads(1, "integration", requests, "Overwrite", client, NewTransferBatch("integration folder")); err != nil {
		t.Fatal(err)
	}

	browseDone := make(chan error, 1)
	go func() {
		for i := 0; i < 10; i++ {
			if _, err := client.ListDirectory(ctx, remoteRoot); err != nil {
				browseDone <- err
				return
			}
		}
		browseDone <- nil
	}()

	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		items := manager.List()
		finished := 0
		for _, item := range items {
			switch item.Status {
			case "Completed":
				finished++
			case "Failed", "Cancelled", "Interrupted":
				cleanupSafe = true
				t.Fatalf("directory upload failed for %s: %s", item.RemotePath, item.Error)
			}
		}
		if finished == files {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	items := manager.List()
	completed := 0
	for _, item := range items {
		if item.Status == "Completed" {
			completed++
		}
	}
	if completed != files {
		statuses := map[string]int{}
		for _, item := range items {
			statuses[item.Status]++
		}
		var state []string
		for status, count := range statuses {
			state = append(state, fmt.Sprintf("%s=%d", status, count))
		}
		cleanupSafe = statuses["Running"]+statuses["Queued"]+statuses["Waiting"] == 0
		t.Fatalf("completed %d of %d directory uploads (%s)", completed, files, strings.Join(state, ", "))
	}
	if err := <-browseDone; err != nil {
		cleanupSafe = true
		t.Fatalf("browse while transferring: %v", err)
	}
	cleanupSafe = true

	seen := make(map[string]map[string]bool)
	found := 0
	for _, remote := range remoteFiles {
		parent := path.Dir(remote)
		if seen[parent] == nil {
			listing, err := client.ListDirectory(ctx, parent)
			if err != nil {
				t.Fatalf("list uploaded directory %s: %v", parent, err)
			}
			seen[parent] = make(map[string]bool, len(listing))
			for _, file := range listing {
				seen[parent][file.Path] = true
			}
		}
		if seen[parent][remote] {
			found++
		}
	}
	if found != files {
		t.Fatalf("parent listings found %d of %d uploaded files", found, files)
	}
	probe := path.Join(remoteRoot, ".git", "objects", "00", fmt.Sprintf("%040x", 0))
	var output bytes.Buffer
	if err := client.Download(ctx, probe, &output); err != nil {
		t.Fatalf("download uploaded file: %v", err)
	}
	if err := client.Rename(ctx, probe, probe+".renamed"); err != nil {
		t.Fatalf("rename uploaded file: %v", err)
	}
	if err := client.Delete(ctx, probe+".renamed"); err != nil {
		t.Fatalf("delete uploaded file: %v", err)
	}
	cleanupRemote()
	cleanupSafe = false
}

// Large payloads exercise TLS shutdown and record buffering that tiny probes miss.
func TestFTPLargeUploadIntegration(t *testing.T) {
	if os.Getenv("QUICKFTP_FTP_INTEGRATION") != "1" {
		t.Skip("set QUICKFTP_FTP_INTEGRATION=1 for a controlled server")
	}
	port, err := strconv.Atoi(os.Getenv("QUICKFTP_FTP_PORT"))
	if err != nil {
		t.Fatal(err)
	}
	client := &FTPClient{Config: FTPConfig{
		Host: os.Getenv("QUICKFTP_FTP_HOST"), Port: port,
		Protocol: storage.Protocol(os.Getenv("QUICKFTP_FTP_PROTOCOL")),
		Username: os.Getenv("QUICKFTP_FTP_USERNAME"), Password: os.Getenv("QUICKFTP_FTP_PASSWORD"),
		TLSVerify: os.Getenv("QUICKFTP_FTP_TLS_VERIFY") != "false", ReuseConnection: true,
	}}
	defer client.Close()
	root := os.Getenv("QUICKFTP_FTP_REMOTE_ROOT")
	if root == "" {
		root = "/site/wwwroot"
	}
	remote := path.Join(root, fmt.Sprintf("quickftp-large-probe-%d.tmp", time.Now().UnixNano()))
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := client.Delete(ctx, remote); err != nil {
			t.Errorf("cleanup large probe %s: %v", remote, err)
		}
	}()
	payload := make([]byte, 8*1024*1024+37)
	for i := range payload {
		payload[i] = byte(i * 37)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for attempt := 1; attempt <= 3; attempt++ {
		if err := client.Upload(ctx, remote, bytes.NewReader(payload)); err != nil {
			t.Fatalf("large protected upload %d: %v", attempt, err)
		}
		var downloaded bytes.Buffer
		if err := client.Download(ctx, remote, &downloaded); err != nil {
			t.Fatalf("verify large upload %d: %v", attempt, err)
		}
		if !bytes.Equal(downloaded.Bytes(), payload) {
			t.Fatalf("large upload %d corrupted: received %d of %d bytes", attempt, downloaded.Len(), len(payload))
		}
	}
}
