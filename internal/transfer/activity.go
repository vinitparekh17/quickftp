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
	"sync/atomic"
	"time"
)

const maxFinishedHistory = 500

var activitySeq atomic.Uint64

type Activity struct {
	ID             string             `json:"id"`
	DeploymentID   int64              `json:"deploymentId,omitempty"`
	BatchID        string             `json:"batchId,omitempty"`
	BatchName      string             `json:"batchName,omitempty"`
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

// TransferBatch identifies a user-requested folder operation across restarts.
type TransferBatch struct{ ID, Name string }

func NewTransferBatch(name string) TransferBatch {
	return TransferBatch{ID: nextActivityID(0), Name: name}
}
func applyTransferBatch(a *Activity, batch []TransferBatch) {
	if len(batch) > 0 && batch[0].ID != "" {
		a.BatchID, a.BatchName = batch[0].ID, batch[0].Name
	} else {
		a.BatchID, a.BatchName = a.ID, path.Base(a.RemotePath)
	}
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
	mu            sync.Mutex
	workers       sync.WaitGroup
	closed        bool
	path          string
	entries       []Activity
	clients       map[string]ActivityClient
	cancels       map[string]context.CancelFunc
	conflicts     map[string]chan string
	Gate          *Gate
	OnChange      func(Activity)
	OnBatchChange func([]Activity)
	OnLog         func(string, string)
}

func OpenActivityManager(file string, gate *Gate) *ActivityManager {
	m := &ActivityManager{
		path:      file,
		Gate:      gate,
		entries:   make([]Activity, 0),
		clients:   make(map[string]ActivityClient),
		cancels:   make(map[string]context.CancelFunc),
		conflicts: make(map[string]chan string),
	}
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
				if isUnfinishedStatus(entry.Status) {
					if entry.Status != "Interrupted" {
						entry.Status = "Interrupted"
						entry.Error = "Application closed before this transfer completed"
					}
					m.entries = append(m.entries, entry)
				} else if entry.Status == "Failed" || entry.Status == "Cancelled" {
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

func isUnfinishedStatus(status string) bool {
	return status == "Queued" || status == "Running" || status == "Waiting" || status == "Interrupted"
}

func isActiveStatus(status string) bool {
	return status == "Queued" || status == "Running" || status == "Waiting"
}

func nextActivityID(connID int64) string {
	seq := activitySeq.Add(1)
	return fmt.Sprintf("%d-%d-%d", time.Now().UnixNano(), connID, seq)
}

func (m *ActivityManager) List() []Activity {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Keep the Wails JSON representation an array when there is no history.
	// A nil slice serializes as null, which breaks frontend consumers that filter it.
	out := make([]Activity, len(m.entries))
	copy(out, m.entries)
	return out
}

func (m *ActivityManager) persistLocked() error {
	if m.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0700); err != nil {
		return err
	}
	unfinished := make([]Activity, 0, len(m.entries))
	for _, entry := range m.entries {
		if entry.Direction != "Deployment upload" && (isUnfinishedStatus(entry.Status) || entry.Status == "Failed" || entry.Status == "Cancelled") {
			unfinished = append(unfinished, entry)
		}
	}
	data, err := json.MarshalIndent(activityFile{Version: 1, Entries: unfinished}, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}

func (m *ActivityManager) prependEntryLocked(a Activity) {
	m.entries = append(m.entries, Activity{})
	copy(m.entries[1:], m.entries[:len(m.entries)-1])
	m.entries[0] = a
	m.pruneHistoryLocked()
}

func (m *ActivityManager) pruneHistoryLocked() {
	if len(m.entries) <= maxFinishedHistory {
		return
	}
	finishedCount := 0
	keep := m.entries[:0]
	for _, entry := range m.entries {
		if isUnfinishedStatus(entry.Status) || (entry.Direction != "Deployment upload" && (entry.Status == "Failed" || entry.Status == "Cancelled")) {
			keep = append(keep, entry)
			continue
		}
		finishedCount++
		if finishedCount <= maxFinishedHistory {
			keep = append(keep, entry)
		}
	}
	for i := len(keep); i < len(m.entries); i++ {
		m.entries[i] = Activity{}
	}
	m.entries = keep
}

func (m *ActivityManager) update(id string, persist bool, fn func(*Activity)) {
	m.mu.Lock()
	var item Activity
	found := false
	for i := range m.entries {
		if m.entries[i].ID == id {
			fn(&m.entries[i])
			item = m.entries[i]
			found = true
			break
		}
	}
	if found && persist {
		m.persistLocked()
	}
	m.mu.Unlock()
	if found && m.OnChange != nil {
		m.OnChange(item)
	}
}

func (m *ActivityManager) AddDownload(connID int64, connName, remote, local string, size int64, policy string, client ActivityClient, batch ...TransferBatch) (Activity, error) {
	if err := safeRemotePath(remote); err != nil {
		return Activity{}, err
	}
	local, err := filepath.Abs(local)
	if err != nil {
		return Activity{}, err
	}
	id := nextActivityID(connID)
	a := Activity{
		ID:             id,
		Direction:      "Download",
		ConnectionID:   connID,
		ConnectionName: connName,
		RemotePath:     path.Clean("/" + remote),
		LocalPath:      local,
		Status:         "Queued",
		Total:          size,
		StartedAt:      time.Now(),
		ConflictPolicy: policy,
	}
	applyTransferBatch(&a, batch)
	if c, ok := client.(*FTPClient); ok {
		a.Profile = snapshotProfile(c.Config)
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Activity{}, errors.New("transfer manager is closed")
	}
	m.workers.Add(1)
	m.prependEntryLocked(a)
	m.clients[id] = client
	ctx, cancel := context.WithCancel(context.Background())
	m.cancels[id] = cancel
	m.persistLocked()
	m.mu.Unlock()
	if m.OnChange != nil {
		m.OnChange(a)
	}
	go m.runDownload(ctx, a)
	return a, nil
}

func prepareUpload(connID int64, connName, remote, local, policy string, client ActivityClient, batch ...TransferBatch) (Activity, error) {
	if err := safeRemotePath(remote); err != nil {
		return Activity{}, err
	}
	abs, err := filepath.Abs(local)
	if err != nil {
		return Activity{}, err
	}
	// Use Lstat so symlinks are not silently followed.
	info, err := os.Lstat(abs)
	if err != nil {
		return Activity{}, err
	}
	if !info.Mode().IsRegular() {
		return Activity{}, errors.New("upload source must be a regular file")
	}
	id := nextActivityID(connID)
	a := Activity{
		ID:             id,
		Direction:      "Upload",
		ConnectionID:   connID,
		ConnectionName: connName,
		RemotePath:     path.Clean("/" + remote),
		LocalPath:      abs,
		Status:         "Queued",
		Total:          info.Size(),
		StartedAt:      time.Now(),
		ConflictPolicy: policy,
	}
	applyTransferBatch(&a, batch)
	if c, ok := client.(*FTPClient); ok {
		a.Profile = snapshotProfile(c.Config)
	}
	return a, nil
}

func (m *ActivityManager) AddUpload(connID int64, connName, remote, local, policy string, client ActivityClient, batch ...TransferBatch) (Activity, error) {
	a, err := prepareUpload(connID, connName, remote, local, policy, client, batch...)
	if err != nil {
		return Activity{}, err
	}
	id := a.ID
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Activity{}, errors.New("transfer manager is closed")
	}
	m.workers.Add(1)
	m.prependEntryLocked(a)
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

// UploadRequest describes one file in a validated directory upload.
type UploadRequest struct{ RemotePath, LocalPath string }

// AddUploads admits a directory in one durable operation. Workers start only
// after the full queue snapshot has been written, avoiding N full-history writes.
func (m *ActivityManager) AddUploads(connID int64, connName string, requests []UploadRequest, policy string, client ActivityClient, batch TransferBatch) ([]Activity, error) {
	entries := make([]Activity, 0, len(requests))
	for _, request := range requests {
		entry, err := prepareUpload(connID, connName, request.RemotePath, request.LocalPath, policy, client, batch)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return entries, nil
	}
	contexts := make([]context.Context, len(entries))
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, errors.New("transfer manager is closed")
	}
	previous := m.entries
	all := make([]Activity, 0, len(entries)+len(m.entries))
	for i := len(entries) - 1; i >= 0; i-- {
		all = append(all, entries[i])
	}
	m.entries = append(all, m.entries...)
	for i, entry := range entries {
		ctx, cancel := context.WithCancel(context.Background())
		contexts[i] = ctx
		m.cancels[entry.ID] = cancel
		m.clients[entry.ID] = client
	}
	m.pruneHistoryLocked()
	if err := m.persistLocked(); err != nil {
		m.entries = previous
		for _, entry := range entries {
			m.cancels[entry.ID]()
			delete(m.cancels, entry.ID)
			delete(m.clients, entry.ID)
		}
		m.mu.Unlock()
		return nil, fmt.Errorf("save directory upload queue: %w", err)
	}
	m.workers.Add(len(entries))
	m.mu.Unlock()
	if m.OnBatchChange != nil {
		m.OnBatchChange(entries)
	} else if m.OnChange != nil {
		for _, entry := range entries {
			m.OnChange(entry)
		}
	}
	workers := 2
	if m.Gate != nil {
		workers = m.Gate.Limit()
	}
	if workers > len(entries) {
		workers = len(entries)
	}
	jobs := make(chan int, len(entries))
	for index := range entries {
		jobs <- index
	}
	close(jobs)
	for range workers {
		go func() {
			for index := range jobs {
				m.runUpload(contexts[index], entries[index])
			}
		}()
	}
	return entries, nil
}

func (m *ActivityManager) runUpload(ctx context.Context, a Activity) {
	defer m.workers.Done()
	m.mu.Lock()
	client := m.clients[a.ID]
	m.mu.Unlock()
	if client == nil {
		m.finish(a.ID, "Failed", errors.New("transfer connection unavailable"))
		return
	}
	defer m.releaseClient(a.ID, client)

	parent := path.Dir(a.RemotePath)
	if parent == "." || parent == "" {
		parent = "/"
	}
	listing, e := client.ListDirectory(ctx, parent)
	if e != nil && !errors.Is(e, ErrNotFound) {
		if ctx.Err() != nil {
			m.finish(a.ID, "Cancelled", ctx.Err())
		} else {
			m.finish(a.ID, "Failed", e)
		}
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
		ch := make(chan string, 1)
		m.mu.Lock()
		m.conflicts[a.ID] = ch
		m.mu.Unlock()
		m.update(a.ID, true, func(x *Activity) {
			x.Status = "Waiting"
			x.Error = "Remote destination already exists"
		})
		m.log("CONFLICT", a.RemotePath+" already exists remotely")
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
	latest := listing
	if didWait {
		var verifyErr error
		latest, verifyErr = client.ListDirectory(ctx, parent)
		if verifyErr != nil {
			m.finish(a.ID, "Failed", fmt.Errorf("could not recheck remote conflict: %w", verifyErr))
			return
		}
	}
	remote := a.RemotePath
	if exists && policy == "Rename" {
		ext := path.Ext(remote)
		base := strings.TrimSuffix(remote, ext)
		for i := 1; ; i++ {
			candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
			found := false
			for _, f := range latest {
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

	m.update(a.ID, true, func(x *Activity) { x.Status = "Running" })
	info, err := os.Lstat(a.LocalPath)
	if err != nil {
		m.finish(a.ID, "Failed", err)
		return
	}
	if !info.Mode().IsRegular() {
		m.finish(a.ID, "Failed", errors.New("upload source is no longer a regular file"))
		return
	}
	f, err := os.Open(a.LocalPath)
	if err != nil {
		m.finish(a.ID, "Failed", err)
		return
	}
	r := &countReader{
		r: f,
		update: func(n int64) {
			m.update(a.ID, false, func(x *Activity) { x.Bytes = n })
			m.log("PROGRESS", fmt.Sprintf("UPLOAD %s — %d / %d bytes", a.RemotePath, n, a.Total))
		},
	}
	err = client.Upload(ctx, remote, r)
	closeErr := f.Close()
	m.update(a.ID, false, func(x *Activity) { x.Bytes = r.n })
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

	verified := false
	if statter, ok := client.(interface {
		FileSize(context.Context, string) (int64, error)
	}); ok {
		remoteSize, sizeErr := statter.FileSize(ctx, remote)
		if sizeErr == nil {
			verified = remoteSize == r.n
		}
		if sizeErr == nil && !verified {
			m.finish(a.ID, "Failed", errors.New("remote size does not match uploaded bytes"))
			return
		}
		if sizeErr != nil {
			// SIZE is not universal; preserve the listing-based verification fallback.
			listing, err = client.ListDirectory(ctx, path.Dir(remote))
			if err != nil {
				m.finish(a.ID, "Failed", fmt.Errorf("upload sent but verification unavailable: %w", err))
				return
			}
			for _, item := range listing {
				if item.Path == remote && !item.IsDir && item.Size == r.n && (item.SizeKnown || r.n == 0) {
					verified = true
					break
				}
			}
		}
	} else {
		listing, err = client.ListDirectory(ctx, path.Dir(remote))
		if err != nil {
			m.finish(a.ID, "Failed", fmt.Errorf("upload sent but verification unavailable: %w", err))
			return
		}
		for _, item := range listing {
			if item.Path == remote && !item.IsDir && item.Size == r.n && (item.SizeKnown || r.n == 0) {
				verified = true
				break
			}
		}
	}

	if !verified {
		m.finish(a.ID, "Failed", errors.New("upload sent, but remote size verification did not pass"))
		return
	}
	m.update(a.ID, false, func(x *Activity) {
		x.RemotePath = remote
		x.Verification = "remote size"
	})
	m.finish(a.ID, "Completed", nil)
}

func (m *ActivityManager) runDownload(ctx context.Context, a Activity) {
	defer m.workers.Done()
	m.mu.Lock()
	client := m.clients[a.ID]
	m.mu.Unlock()
	if client == nil {
		m.finish(a.ID, "Failed", errors.New("transfer connection unavailable"))
		return
	}
	defer m.releaseClient(a.ID, client)

	if err := os.MkdirAll(filepath.Dir(a.LocalPath), 0700); err != nil {
		m.finish(a.ID, "Failed", err)
		return
	}
	policy := a.ConflictPolicy
	if policy == "" {
		policy = "Ask"
	}
	exists := false
	if _, err := os.Lstat(a.LocalPath); err == nil {
		exists = true
		if policy == "Ask" {
			ch := make(chan string, 1)
			m.mu.Lock()
			m.conflicts[a.ID] = ch
			m.mu.Unlock()
			m.update(a.ID, true, func(x *Activity) {
				x.Status = "Waiting"
				x.Error = "Destination already exists"
			})
			m.log("CONFLICT", a.RemotePath+" → "+a.LocalPath)
			select {
			case policy = <-ch:
			case <-ctx.Done():
				m.finish(a.ID, "Cancelled", ctx.Err())
				return
			}
		}
	}
	if exists && policy == "Skip" {
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

	m.update(a.ID, true, func(x *Activity) { x.Status = "Running" })
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
	count := &countWriter{
		w: f,
		update: func(n int64) {
			m.update(a.ID, false, func(x *Activity) { x.Bytes = n })
			m.log("PROGRESS", fmt.Sprintf("DOWNLOAD %s — %d / %d bytes", a.RemotePath, n, a.Total))
		},
	}
	err = client.Download(ctx, a.RemotePath, count)
	closeErr := f.Close()
	m.update(a.ID, false, func(x *Activity) { x.Bytes = count.n })
	if err == nil {
		err = closeErr
	}
	if err == nil && a.Total > 0 && count.n != a.Total {
		err = fmt.Errorf("download size mismatch: expected %d bytes, received %d", a.Total, count.n)
	}
	if err == nil {
		var publishErr error
		if policy == "Overwrite" {
			if info, statErr := os.Lstat(target); statErr == nil && info.IsDir() {
				publishErr = errors.New("cannot overwrite a directory with a file")
			} else {
				publishErr = os.Rename(tmp, target)
			}
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
			m.update(a.ID, false, func(x *Activity) {
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
	m.mu.Lock()
	var item Activity
	found := false
	for i := range m.entries {
		if m.entries[i].ID == id {
			m.entries[i].Status = status
			m.entries[i].FinishedAt = time.Now()
			if err != nil {
				m.entries[i].Error = err.Error()
			} else {
				m.entries[i].Error = ""
			}
			item = m.entries[i]
			found = true
			break
		}
	}
	if cancel, ok := m.cancels[id]; ok {
		cancel()
		delete(m.cancels, id)
	}
	delete(m.clients, id)
	delete(m.conflicts, id)
	m.pruneHistoryLocked()
	if !m.closed {
		m.persistLocked()
	}
	m.mu.Unlock()

	if found && m.OnChange != nil {
		m.OnChange(item)
	}
	level := "SUCCESS"
	detail := status
	if err != nil {
		level = "ERROR"
		detail = err.Error()
	}
	if found {
		m.log(level, fmt.Sprintf("%s %s — %s: %s", item.Direction, item.RemotePath, status, detail))
	}
}

// releaseClient closes one-shot clients when their activity ends. Directory
// uploads share an FTPClient across many activities, so keep that connection
// alive until the last activity using it has finished.
func (m *ActivityManager) releaseClient(id string, client ActivityClient) {
	if client == nil {
		return
	}
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
		closed := m.closed
		m.mu.Unlock()
		if closed {
			_ = ftpClient.Close()
			return
		}
		if inUse {
			return
		}
		ftpClient.CloseWhenIdle(30 * time.Second)
		return
	}
	_ = closer.Close()
}

// Close stops admission and waits for workers to release files and connections.
func (m *ActivityManager) Close() {
	m.mu.Lock()
	m.closed = true
	for _, cancel := range m.cancels {
		cancel()
	}
	m.mu.Unlock()
	m.workers.Wait()
	m.mu.Lock()
	m.persistLocked()
	m.mu.Unlock()
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
	case "Skip", "Rename", "Overwrite":
	default:
		return errors.New("invalid conflict resolution policy")
	}
	m.mu.Lock()
	ch := m.conflicts[id]
	delete(m.conflicts, id)
	m.mu.Unlock()
	if ch == nil {
		return errors.New("transfer is not waiting for a conflict decision")
	}
	m.update(id, true, func(x *Activity) { x.Error = "" })
	select {
	case ch <- policy:
	default:
	}
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
	if old.ID == "" || (old.Status != "Failed" && old.Status != "Interrupted" && old.Status != "Cancelled") {
		return errors.New("transfer cannot be retried")
	}
	var err error
	if old.Direction == "Upload" {
		_, err = m.AddUpload(old.ConnectionID, old.ConnectionName, old.RemotePath, old.LocalPath, old.ConflictPolicy, client, TransferBatch{ID: old.BatchID, Name: old.BatchName})
	} else {
		_, err = m.AddDownload(old.ConnectionID, old.ConnectionName, old.RemotePath, old.LocalPath, old.Total, old.ConflictPolicy, client, TransferBatch{ID: old.BatchID, Name: old.BatchName})
	}
	if err == nil {
		m.Remove(id)
	}
	return err
}

func snapshotProfile(c FTPConfig) ConnectionSnapshot {
	return ConnectionSnapshot{
		Host:          c.Host,
		Port:          c.Port,
		Protocol:      string(c.Protocol),
		Username:      c.Username,
		TLSVerify:     c.TLSVerify,
		CredentialKey: c.CredentialKey,
	}
}

// ClearHistory removes only finished manual transfers, optionally for one connection.
// It never cancels active work or deletes remote files, deployments, or backups.
func (m *ActivityManager) ClearHistory(connectionID int64) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := []string{}
	previous := m.entries
	kept := make([]Activity, 0, len(m.entries))
	for _, entry := range m.entries {
		if entry.Direction != "Deployment upload" && !isActiveStatus(entry.Status) && (connectionID == 0 || entry.ConnectionID == connectionID) {
			removed = append(removed, entry.ID)
		} else {
			kept = append(kept, entry)
		}
	}
	m.entries = kept
	if err := m.persistLocked(); err != nil {
		m.entries = previous
		return nil, err
	}
	return removed, nil
}

func (m *ActivityManager) Remove(id string) {
	m.mu.Lock()
	out := m.entries[:0]
	for _, x := range m.entries {
		if x.ID != id || isActiveStatus(x.Status) {
			out = append(out, x)
		}
	}
	for i := len(out); i < len(m.entries); i++ {
		m.entries[i] = Activity{}
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
	id := fmt.Sprintf("deploy-%d-%s", deploymentID, remote)
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
		out = Activity{
			ID:             id,
			Direction:      "Deployment upload",
			DeploymentID:   deploymentID,
			ConnectionID:   connectionID,
			ConnectionName: connectionName,
			RemotePath:     remote,
			LocalPath:      local,
			Status:         status,
			Bytes:          bytes,
			Total:          total,
			StartedAt:      now,
			Error:          detail,
		}
		if status == "Completed" || status == "Failed" {
			out.FinishedAt = now
		}
		m.prependEntryLocked(out)
	}
	m.mu.Unlock()
	if m.OnChange != nil {
		m.OnChange(out)
	}
	switch {
	case status == "Queued":
		m.log("START", "UPLOAD "+remote)
	case status == "Running" && bytes > 0:
		m.log("PROGRESS", fmt.Sprintf("UPLOAD %s — %d / %d bytes", remote, bytes, total))
	case status == "Completed":
		m.log("SUCCESS", "UPLOAD "+remote+" completed and verified")
	case status == "Failed":
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
	if n > 0 {
		w.n += int64(n)
		if w.update != nil && (w.n-w.lastN >= 512*1024 || time.Since(w.lastAt) >= 500*time.Millisecond) {
			w.update(w.n)
			w.lastN, w.lastAt = w.n, time.Now()
		}
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
	if n > 0 {
		r.n += int64(n)
		if r.update != nil && (r.n-r.lastN >= 512*1024 || time.Since(r.lastAt) >= 500*time.Millisecond) {
			r.update(r.n)
			r.lastN, r.lastAt = r.n, time.Now()
		}
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
	if p == "" || strings.ContainsRune(p, 0) || strings.Contains(p, "\\") {
		return errors.New("unsafe remote path")
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return errors.New("remote path traversal rejected")
		}
	}
	return nil
}
