package transfer

import (
	"container/list"
	"context"
)

// Match the largest transfer limit supported by Gate. The gate still controls
// the actual number of simultaneous transfers, including uploads and deployments.
const maxDownloadWorkers = 5

type downloadJob struct {
	activity Activity
	ctx      context.Context
	element  *list.Element
	running  bool
	waiting  bool
	policy   string
}

func (m *ActivityManager) scheduleDownloadLocked(job *downloadJob) {
	job.waiting = false
	job.element = m.downloadQueue.PushBack(job)
	if m.downloadWorkers < maxDownloadWorkers {
		m.downloadWorkers++
		m.workers.Add(1)
		go m.workDownloads()
	}
}

func (m *ActivityManager) workDownloads() {
	defer m.workers.Done()
	for {
		m.mu.Lock()
		next := m.downloadQueue.Front()
		if next == nil || m.closed {
			m.downloadWorkers--
			m.mu.Unlock()
			return
		}
		job := next.Value.(*downloadJob)
		m.downloadQueue.Remove(next)
		job.element = nil
		job.running = true
		id := job.activity.ID
		client := m.clients[id]
		m.mu.Unlock()

		m.runDownload(job, client)

		m.mu.Lock()
		// A finished transfer has already been removed. Conflicts park the
		// job here, freeing the worker while the user decides what to do.
		parked := m.downloads[id] != nil
		if parked && !m.closed && job.ctx.Err() == nil {
			job.running = false
			if job.policy != "" {
				m.scheduleDownloadLocked(job)
			}
			m.mu.Unlock()
			continue
		}
		m.mu.Unlock()
		if parked {
			m.finish(id, "Cancelled", context.Canceled)
		}
		m.releaseClient(id, client)
	}
}

// removePendingDownloadLocked claims a queued or parked download for cancellation.
func (m *ActivityManager) removePendingDownloadLocked(id string) bool {
	job := m.downloads[id]
	if job == nil || job.running {
		return false
	}
	if job.element != nil {
		m.downloadQueue.Remove(job.element)
	}
	delete(m.downloads, id)
	return true
}

func (m *ActivityManager) cancelPendingDownload(id string) {
	m.mu.Lock()
	client := m.clients[id]
	m.mu.Unlock()
	m.finish(id, "Cancelled", context.Canceled)
	m.releaseClient(id, client)
}
