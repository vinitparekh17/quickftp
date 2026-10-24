package transfer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Activity struct {
	ID             string             `json:"id"`
	Direction      string             `json:"direction"`
	ConnectionID   int64              `json:"connectionId"`
	ConnectionName string             `json:"connectionName"`
	RemotePath     string             `json:"remotePath"`
	LocalPath      string             `json:"localPath"`
	Status         string             `json:"status"`
	Bytes          int64              `json:"bytes"`
	Total          int64              `json:"total"`
	StartedAt      time.Time          `json:"startedAt"`
	FinishedAt     time.Time          `json:"finishedAt,omitempty"`
	Error          string             `json:"error,omitempty"`
	Verification   string             `json:"verification,omitempty"`
	ConflictPolicy string             `json:"conflictPolicy,omitempty"`
	Profile        ConnectionSnapshot `json:"profile"`
}
type ConnectionSnapshot struct {
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Protocol      string `json:"protocol"`
	Username      string `json:"username"`
	TLSVerify     bool   `json:"tlsVerify"`
	CredentialKey string `json:"credentialKey"`
}
type activityFile struct {
	Version int        `json:"version"`
	Entries []Activity `json:"entries"`
}
type ActivityClient interface {
	ListDirectory(context.Context, string) ([]RemoteFile, error)
	Download(context.Context, string, io.Writer) error
	Upload(context.Context, string, io.Reader) error
}
type ActivityManager struct {
	mu        sync.Mutex
	path      string
	entries   []Activity
	clients   map[string]ActivityClient
	cancels   map[string]context.CancelFunc
	conflicts map[string]chan string
	Gate      *Gate
	OnChange  func(Activity)
	OnLog     func(string, string)
}

func OpenActivityManager(file string, gate *Gate) *ActivityManager {
	m := &ActivityManager{path: file, Gate: gate, clients: map[string]ActivityClient{}, cancels: map[string]context.CancelFunc{}, conflicts: map[string]chan string{}}
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		// Import unfinished work from the previous MVP file name once; completed
		// transfer history is intentionally discarded on this migration.
		data, err = os.ReadFile(filepath.Join(filepath.Dir(file), "transfers.json"))
	}
	if err == nil {
		var v activityFile
		if json.Unmarshal(data, &v) == nil {
			for _, entry := range v.Entries {
				if entry.Status == "Queued" || entry.Status == "Running" || entry.Status == "Waiting" || entry.Status == "Interrupted" {
					entry.Status = "Interrupted"
					entry.Error = "Application closed before this transfer completed"
					m.entries = append(m.entries, entry)
				}
			}
		}
	}
	for i := range m.entries {
		if m.entries[i].Direction == "Download" && m.entries[i].Status == "Interrupted" {
			_ = os.Remove(m.entries[i].LocalPath + ".quickftp-" + m.entries[i].ID + ".partial")
		}
	}
	m.persistLocked()
	return m
}
func (m *ActivityManager) List() []Activity {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Keep the Wails JSON representation an array when there is no history.
	// A nil slice serializes as null, which breaks frontend consumers that filter it.
	return append([]Activity{}, m.entries...)
}
func (m *ActivityManager) persistLocked() {
	_ = os.MkdirAll(filepath.Dir(m.path), 0700)
	// Persist only unfinished work needed to surface crash recovery next launch.
	unfinished := make([]Activity, 0)
	for _, entry := range m.entries {
		if entry.Status == "Queued" || entry.Status == "Running" || entry.Status == "Waiting" || entry.Status == "Interrupted" {
			unfinished = append(unfinished, entry)
		}
	}
	b, e := json.MarshalIndent(activityFile{Version: 1, Entries: unfinished}, "", "  ")
	if e != nil {
		return
	}
	tmp := m.path + ".tmp"
	if os.WriteFile(tmp, b, 0600) == nil {
		_ = os.Rename(tmp, m.path)
	}
}
func (m *ActivityManager) update(id string, fn func(*Activity)) {
	m.mu.Lock()
	var copy Activity
	for i := range m.entries {
		if m.entries[i].ID == id {
			fn(&m.entries[i])
			copy = m.entries[i]
			break
		}
	}
	m.persistLocked()
	m.mu.Unlock()
	if m.OnChange != nil {
		m.OnChange(copy)
	}
}
func (m *ActivityManager) AddDownload(connID int64, connName, remote, local string, size int64, policy string, client ActivityClient) (Activity, error) {
	if err := safeRemotePath(remote); err != nil {
		return Activity{}, err
	}
	local, err := filepath.Abs(local)
	if err != nil {
		return Activity{}, err
	}
	id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), connID)
	a := Activity{ID: id, Direction: "Download", ConnectionID: connID, ConnectionName: connName, RemotePath: remote, LocalPath: local, Status: "Queued", Total: size, StartedAt: time.Now()}
	if c, ok := client.(*FTPClient); ok {
		a.Profile = snapshotProfile(c.Config)
	}
	m.mu.Lock()
	m.entries = append([]Activity{a}, m.entries...)
	m.clients[id] = client
	ctx, cancel := context.WithCancel(context.Background())
	m.cancels[id] = cancel
	m.persistLocked()
	m.mu.Unlock()
	if m.OnChange != nil {
		m.OnChange(a)
	}
	a.ConflictPolicy = policy
	go m.runDownload(ctx, a)
	return a, nil
}

func (m *ActivityManager) AddUpload(connID int64, connName, remote, local, policy string, client ActivityClient) (Activity, error) {
	if err := safeRemotePath(remote); err != nil {
		return Activity{}, err
	}
	abs, err := filepath.Abs(local)
	if err != nil {
		return Activity{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Activity{}, err
	}
	if !info.Mode().IsRegular() {
		return Activity{}, errors.New("upload source must be a regular file")
	}
	id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), connID)
	a := Activity{ID: id, Direction: "Upload", ConnectionID: connID, ConnectionName: connName, RemotePath: remote, LocalPath: abs, Status: "Queued", Total: info.Size(), StartedAt: time.Now(), ConflictPolicy: policy}
	if c, ok := client.(*FTPClient); ok {
		a.Profile = snapshotProfile(c.Config)
	}
	m.mu.Lock()
	m.entries = append([]Activity{a}, m.entries...)
	m.clients[id] = client
	ctx, cancel := context.WithCancel(context.Background())
	m.cancels[id] = cancel
	m.persistLocked()
	m.mu.Unlock()
	if m.OnChange != nil {
		m.OnChange(a)
	}
	go m.runUpload(ctx, a)
	return a, nil
}
func (m *ActivityManager) runUpload(ctx context.Context, a Activity) {
	m.mu.Lock()
	client := m.clients[a.ID]
	m.mu.Unlock()
	if client == nil {
		m.finish(a.ID, "Failed", errors.New("transfer connection unavailable"))
		return
	}
	defer m.releaseClient(a.ID, client)
	parent := filepath.ToSlash(filepath.Dir(a.RemotePath))
	if parent == "." {
		parent = "/"
	}
	listing, e := client.ListDirectory(ctx, parent)
	if e != nil {
		m.finish(a.ID, "Failed", e)
		return
	}
	exists := false
	for _, f := range listing {
		if f.Path == a.RemotePath {
			exists = true
			break
		}
	}
	policy := a.ConflictPolicy
	if policy == "" {
		policy = "Ask"
	}
	didWait := false
	if exists && policy == "Ask" {
		didWait = true
		m.update(a.ID, func(x *Activity) { x.Status = "Waiting"; x.Error = "Remote destination already exists" })
		m.log("CONFLICT", a.RemotePath+" already exists remotely")
		ch := make(chan string, 1)
		m.mu.Lock()
		m.conflicts[a.ID] = ch
		m.mu.Unlock()
		select {
		case policy = <-ch:
		case <-ctx.Done():
			m.finish(a.ID, "Cancelled", ctx.Err())
			return
		}
	}
	if exists && policy == "Skip" {
		m.finish(a.ID, "Skipped", nil)
		return
	}
	remote := a.RemotePath
	if exists && policy == "Rename" {
		ext := path.Ext(remote)
		base := strings.TrimSuffix(remote, ext)
		for i := 1; ; i++ {
			candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
			found := false
			for _, f := range listing {
				if f.Path == candidate {
					found = true
					break
				}
			}
			if !found {
				remote = candidate
				break
			}
		}
	}
	latest := listing
	if didWait {
		var verifyErr error
		latest, verifyErr = client.ListDirectory(ctx, parent)
		if verifyErr != nil {
			m.finish(a.ID, "Failed", fmt.Errorf("could not recheck remote conflict: %w", verifyErr))
			return
		}
	}
	for _, f := range latest {
		if f.Path == remote && policy != "Overwrite" {
			m.finish(a.ID, "Failed", errors.New("remote destination appeared before upload; retry to resolve conflict"))
			return
		}
	}
	release := func() {}
	if m.Gate != nil {
		release, e = m.Gate.Acquire(ctx)
		if e != nil {
			m.finish(a.ID, "Cancelled", e)
			return
		}
	}
	defer release()
	m.update(a.ID, func(x *Activity) { x.Status = "Running" })
	f, err := os.Open(a.LocalPath)
	if err != nil {
		m.finish(a.ID, "Failed", err)
		return
	}
	r := &countReader{r: f, update: func(n int64) {
		m.update(a.ID, func(x *Activity) { x.Bytes = n })
		m.log("PROGRESS", fmt.Sprintf("UPLOAD %s — %d / %d bytes", a.RemotePath, n, a.Total))
	}}
	err = client.Upload(ctx, remote, r)
	closeErr := f.Close()
	m.update(a.ID, func(x *Activity) { x.Bytes = r.n })
	if err == nil {
		err = closeErr
	}
	if err != nil {
		if ctx.Err() != nil {
			m.finish(a.ID, "Cancelled", ctx.Err())
		} else {
			m.finish(a.ID, "Failed", err)
		}
		return
	}
	parent = filepath.ToSlash(path.Dir(remote))
	if parent == "." {
		parent = "/"
	}
	listing, err = client.ListDirectory(ctx, parent)
	if err != nil {
		m.finish(a.ID, "Failed", fmt.Errorf("upload sent but verification unavailable: %w", err))
		return
	}
	verified := false
	for _, f := range listing {
		if f.Path == remote && f.SizeKnown && f.Size == a.Total {
			verified = true
			break
		}
	}
	if !verified {
		m.finish(a.ID, "Failed", errors.New("upload sent, but remote size verification did not pass"))
		return
	}
	m.update(a.ID, func(x *Activity) { x.RemotePath = remote; x.Verification = "remote size" })
	m.finish(a.ID, "Completed", nil)
}
func (m *ActivityManager) runDownload(ctx context.Context, a Activity) {
	m.mu.Lock()
	client := m.clients[a.ID]
	m.mu.Unlock()
	defer m.releaseClient(a.ID, client)
	if err := os.MkdirAll(filepath.Dir(a.LocalPath), 0700); err != nil {
		m.finish(a.ID, "Failed", err)
		return
	}
	policy := a.ConflictPolicy
	if policy == "" {
		policy = "Ask"
	}
	if _, err := os.Lstat(a.LocalPath); err == nil {
		if policy == "Ask" {
			m.update(a.ID, func(x *Activity) { x.Status = "Waiting"; x.Error = "Destination already exists" })
			m.log("CONFLICT", a.RemotePath+" → "+a.LocalPath)
			ch := make(chan string, 1)
			m.mu.Lock()
			m.conflicts[a.ID] = ch
			m.mu.Unlock()
			select {
			case policy = <-ch:
			case <-ctx.Done():
				m.finish(a.ID, "Cancelled", ctx.Err())
				return
			}
		}
	}
	if policy == "Skip" {
		m.finish(a.ID, "Skipped", nil)
		return
	}
	release := func() {}
	if m.Gate != nil {
		var e error
		release, e = m.Gate.Acquire(ctx)
		if e != nil {
			m.finish(a.ID, "Cancelled", e)
			return
		}
	}
	defer release()
	m.update(a.ID, func(x *Activity) { x.Status = "Running" })
	target := a.LocalPath
	if policy == "Rename" {
		target = uniqueLocalName(target)
	}
	if policy != "Overwrite" && policy != "Rename" {
		if _, e := os.Lstat(target); e == nil {
			m.finish(a.ID, "Failed", errors.New("destination exists; resolve conflict and retry"))
			return
		}
	}
	tmp := target + ".quickftp-" + a.ID + ".partial"
	_ = os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		m.finish(a.ID, "Failed", err)
		return
	}
	count := &countWriter{w: f, update: func(n int64) {
		m.update(a.ID, func(x *Activity) { x.Bytes = n })
		m.log("PROGRESS", fmt.Sprintf("DOWNLOAD %s — %d / %d bytes", a.RemotePath, n, a.Total))
	}}
	if client == nil {
		err = errors.New("transfer connection unavailable")
	} else {
		err = client.Download(ctx, a.RemotePath, count)
	}
	closeErr := f.Close()
	m.update(a.ID, func(x *Activity) { x.Bytes = count.n })
	if err == nil {
		err = closeErr
	}
	if err == nil && a.Total > 0 && count.n != a.Total {
		err = fmt.Errorf("download size mismatch: expected %d bytes, received %d", a.Total, count.n)
	}
	if err == nil {
		var publishErr error
		if policy == "Overwrite" {
			_ = os.Remove(target)
			publishErr = os.Rename(tmp, target)
		} else {
			if _, statErr := os.Lstat(target); statErr == nil {
				publishErr = errors.New("destination already exists")
			} else {
				publishErr = os.Rename(tmp, target)
			}
		}
		if publishErr != nil {
			err = publishErr
		} else {
			m.update(a.ID, func(x *Activity) {
				x.LocalPath = target
				x.Verification = "size"
				if a.Total == 0 {
					x.Verification = "transfer-complete; size unavailable"
				}
			})
		}
	}
	if err != nil {
		_ = os.Remove(tmp)
		if ctx.Err() != nil {
			m.finish(a.ID, "Cancelled", ctx.Err())
		} else {
			m.finish(a.ID, "Failed", err)
		}
		return
	}
	m.finish(a.ID, "Completed", nil)
}
func (m *ActivityManager) finish(id, status string, err error) {
	var item Activity
	for _, x := range m.List() {
		if x.ID == id {
			item = x
			break
		}
	}
	m.update(id, func(x *Activity) {
		x.Status = status
		x.FinishedAt = time.Now()
		if err != nil {
			x.Error = err.Error()
		} else {
			x.Error = ""
		}
	})
	m.mu.Lock()
	delete(m.cancels, id)
	delete(m.clients, id)
	delete(m.conflicts, id)
	m.mu.Unlock()
	level := "SUCCESS"
	detail := status
	if err != nil {
		level = "ERROR"
		detail = err.Error()
	}
	m.log(level, fmt.Sprintf("%s %s — %s: %s", item.Direction, item.RemotePath, status, detail))
}

// releaseClient closes one-shot clients when their activity ends. Directory
// uploads share an FTPClient across many activities, so keep that connection
// alive until the last activity using it has finished.
func (m *ActivityManager) releaseClient(id string, client ActivityClient) {
	closer, ok := client.(io.Closer)
	if !ok {
		return
	}
	if ftpClient, ok := client.(*FTPClient); ok {
		m.mu.Lock()
		delete(m.clients, id)
		inUse := false
		for _, other := range m.clients {
			if otherFTP, ok := other.(*FTPClient); ok && otherFTP == ftpClient {
				inUse = true
				break
			}
		}
		m.mu.Unlock()
		if inUse {
			return
		}
		ftpClient.CloseWhenIdle(30 * time.Second)
		return
	}
	_ = closer.Close()
}

func (m *ActivityManager) Cancel(id string) error {
	m.mu.Lock()
	c := m.cancels[id]
	m.mu.Unlock()
	if c == nil {
		return errors.New("transfer is not active")
	}
	c()
	return nil
}
func (m *ActivityManager) ResolveConflict(id, policy string) error {
	switch policy {
	case "Ask", "Skip", "Rename", "Overwrite":
	default:
		return errors.New("invalid conflict policy")
	}
	m.mu.Lock()
	ch := m.conflicts[id]
	m.mu.Unlock()
	if ch == nil {
		return errors.New("transfer is not waiting for a conflict decision")
	}
	m.update(id, func(x *Activity) { x.Error = "" })
	ch <- policy
	return nil
}
func (m *ActivityManager) Retry(id string, client ActivityClient) error {
	m.mu.Lock()
	var old Activity
	for _, x := range m.entries {
		if x.ID == id {
			old = x
			break
		}
	}
	m.mu.Unlock()
	if old.ID == "" || old.Status != "Failed" && old.Status != "Interrupted" && old.Status != "Cancelled" {
		return errors.New("transfer cannot be retried")
	}
	var e error
	if old.Direction == "Upload" {
		_, e = m.AddUpload(old.ConnectionID, old.ConnectionName, old.RemotePath, old.LocalPath, old.ConflictPolicy, client)
	} else {
		_, e = m.AddDownload(old.ConnectionID, old.ConnectionName, old.RemotePath, old.LocalPath, old.Total, old.ConflictPolicy, client)
	}
	return e
}
func snapshotProfile(c FTPConfig) ConnectionSnapshot {
	return ConnectionSnapshot{Host: c.Host, Port: c.Port, Protocol: string(c.Protocol), Username: c.Username, TLSVerify: c.TLSVerify, CredentialKey: c.CredentialKey}
}
func (m *ActivityManager) Remove(id string) {
	m.mu.Lock()
	out := m.entries[:0]
	for _, x := range m.entries {
		if x.ID != id || x.Status == "Running" || x.Status == "Queued" || x.Status == "Waiting" {
			out = append(out, x)
		}
	}
	m.entries = out
	m.persistLocked()
	m.mu.Unlock()
}
func (m *ActivityManager) log(level, msg string) {
	if m.OnLog != nil {
		m.OnLog(level, msg)
	}
}
func (m *ActivityManager) RecordLog(level, msg string) { m.log(level, msg) }

func (m *ActivityManager) RecordDeploymentTransfer(deploymentID, connectionID int64, connectionName, remote, local string, total, bytes int64, status, detail string) {
	id := fmt.Sprintf("deploy-%d-%s", deploymentID, strings.ReplaceAll(strings.Trim(remote, "/"), "/", "_"))
	now := time.Now()
	var out Activity
	m.mu.Lock()
	found := false
	for i := range m.entries {
		if m.entries[i].ID == id {
			found = true
			m.entries[i].Bytes = bytes
			m.entries[i].Status = status
			m.entries[i].Error = detail
			if status == "Completed" || status == "Failed" {
				m.entries[i].FinishedAt = now
			}
			out = m.entries[i]
			break
		}
	}
	if !found {
		out = Activity{ID: id, Direction: "Deployment upload", ConnectionID: connectionID, ConnectionName: connectionName, RemotePath: remote, LocalPath: local, Status: status, Bytes: bytes, Total: total, StartedAt: now, Error: detail}
		if status == "Completed" || status == "Failed" {
			out.FinishedAt = now
		}
		m.entries = append([]Activity{out}, m.entries...)
	}
	m.persistLocked()
	m.mu.Unlock()
	if m.OnChange != nil {
		m.OnChange(out)
	}
	if status == "Queued" {
		m.log("START", "UPLOAD "+remote)
	} else if status == "Running" && bytes > 0 {
		m.log("PROGRESS", fmt.Sprintf("UPLOAD %s — %d / %d bytes", remote, bytes, total))
	} else if status == "Completed" {
		m.log("SUCCESS", "UPLOAD "+remote+" completed and verified")
	} else if status == "Failed" {
		m.log("ERROR", "UPLOAD "+remote+": "+detail)
	}
}

type countWriter struct {
	w      io.Writer
	n      int64
	update func(int64)
	lastN  int64
	lastAt time.Time
}

func (w *countWriter) Write(p []byte) (int, error) {
	n, e := w.w.Write(p)
	w.n += int64(n)
	if w.update != nil && (w.n-w.lastN >= 512*1024 || time.Since(w.lastAt) >= 500*time.Millisecond) {
		w.update(w.n)
		w.lastN, w.lastAt = w.n, time.Now()
	}
	return n, e
}

type countReader struct {
	r      io.Reader
	n      int64
	update func(int64)
	lastN  int64
	lastAt time.Time
}

func (r *countReader) Read(p []byte) (int, error) {
	n, e := r.r.Read(p)
	r.n += int64(n)
	if r.update != nil && (r.n-r.lastN >= 512*1024 || time.Since(r.lastAt) >= 500*time.Millisecond) {
		r.update(r.n)
		r.lastN, r.lastAt = r.n, time.Now()
	}
	return n, e
}
func uniqueLocalName(p string) string {
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 1; ; i++ {
		n := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, e := os.Lstat(n); os.IsNotExist(e) {
			return n
		}
	}
}
func safeRemotePath(p string) error {
	if strings.ContainsRune(p, 0) || strings.Contains(p, "\\") {
		return errors.New("unsafe remote path")
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return errors.New("remote path traversal rejected")
		}
	}
	return nil
}
