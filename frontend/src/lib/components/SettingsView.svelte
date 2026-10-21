<script lang="ts">
  import Icon from './Icon.svelte';
  import appIcon from '../../assets/quickftp-icon.png';
  import { onMount } from 'svelte';
  import { addToast } from '../stores';
  import { getSettings, saveSettings, selectDirectory } from '../wailsjs';

  let backupDir = '%LOCALAPPDATA%\\QuickFTP\\backups';
  let tempDir = '%TEMP%\\QuickFTP';
  let retention = 'all';
  let downloadDir = '';
  let downloadConflict = 'Ask';
  let uploadConflict = 'Ask';
  let maxConcurrent = 2;
  let saving = false;

  onMount(async () => {
    try {
      const s = await getSettings();
      backupDir = s.backupDir;
      downloadDir = s.downloadDir;
      downloadConflict = s.downloadConflict;
      uploadConflict = s.uploadConflict;
      maxConcurrent = s.maxConcurrent;
    } catch {
      backupDir = localStorage.getItem('quickftp.backupDir') ?? backupDir;
    }
    tempDir = localStorage.getItem('quickftp.tempDir') ?? tempDir;
    retention = localStorage.getItem('quickftp.retention') ?? retention;
  });

  async function save() {
    saving = true;
    try {
      await saveSettings({
        backupDir,
        downloadDir,
        downloadConflict,
        uploadConflict,
        maxConcurrent,
      });
      localStorage.setItem('quickftp.backupDir', backupDir);
      localStorage.setItem('quickftp.tempDir', tempDir);
      localStorage.setItem('quickftp.retention', retention);
      addToast('success', 'Preferences saved successfully');
    } catch (err: any) {
      addToast('error', `Could not save settings: ${err.message}`);
    } finally {
      saving = false;
    }
  }

  async function browseBackupDir() {
    try {
      const selected = await selectDirectory('Select backup directory');
      if (selected) backupDir = selected;
    } catch (err: any) {
      addToast('error', `Could not open directory picker: ${err.message}`);
    }
  }

  async function browseDownloadDir() {
    try {
      const selected = await selectDirectory('Select download directory');
      if (selected) downloadDir = selected;
    } catch (err: any) {
      addToast('error', `Could not open directory picker: ${err.message}`);
    }
  }
</script>

<div class="view-full-height">
  <section class="page-heading">
  <div>
    <p class="eyebrow">PREFERENCES</p>
    <h1>Settings</h1>
    <p class="page-subtitle">Configure local directories, conflict resolutions, and application policies.</p>
  </div>
  <button class="btn btn-primary" on:click={save} disabled={saving}>
    <Icon name="check" size={16} />
    <span>{saving ? 'Saving...' : 'Save Settings'}</span>
  </button>
</section>

<div class="settings-container">
  <!-- Section 1: Storage Locations -->
  <div class="card mb-4">
    <div class="card-header">
      <div class="card-title-group">
        <Icon name="folder" size={18} />
        <div>
          <h2>Storage Locations</h2>
          <p class="text-sm text-muted">Manage directory paths for backups, downloads, and scratch cache.</p>
        </div>
      </div>
    </div>
    <div class="card-body">
      <div class="form-group">
        <label class="form-label" for="setting-backup-dir">Remote Backup Directory</label>
        <div class="path-control">
          <input
            id="setting-backup-dir"
            class="form-input"
            aria-label="Default backup directory"
            bind:value={backupDir}
            placeholder="C:\Users\...\AppData\Local\QuickFTP\backups"
          />
          <button class="btn btn-secondary" type="button" on:click={browseBackupDir}>
            <Icon name="folder" size={14} />
            <span>Browse…</span>
          </button>
        </div>
        <div class="form-hint">
          Local directory where verified remote file snapshots are stored prior to staging and replacement.
        </div>
      </div>

      <div class="form-group">
        <label class="form-label" for="setting-download-dir">Default Download Directory</label>
        <div class="path-control">
          <input
            id="setting-download-dir"
            class="form-input"
            aria-label="Default download directory"
            bind:value={downloadDir}
            placeholder="C:\Users\...\Downloads\QuickFTP"
          />
          <button class="btn btn-secondary" type="button" on:click={browseDownloadDir}>
            <Icon name="folder" size={14} />
            <span>Browse…</span>
          </button>
        </div>
        <div class="form-hint">
          Destination folder on your machine when downloading files from the File Explorer.
        </div>
      </div>

      <div class="form-group" style="margin-bottom: 0;">
        <label class="form-label" for="setting-temp-dir">Temporary Scratch Directory</label>
        <input
          id="setting-temp-dir"
          class="form-input"
          aria-label="Temporary file directory"
          bind:value={tempDir}
          placeholder="%TEMP%\QuickFTP"
        />
        <div class="form-hint">
          Used for intermediate file verification and temporary staging during build and transfer pipelines.
        </div>
      </div>
    </div>
  </div>

  <!-- Section 2: Transfer Policies & Concurrency -->
  <div class="card mb-4">
    <div class="card-header">
      <div class="card-title-group">
        <Icon name="transfer" size={18} />
        <div>
          <h2>Transfer &amp; Collision Policy</h2>
          <p class="text-sm text-muted">Configure default collision handling rules and concurrency limits.</p>
        </div>
      </div>
    </div>
    <div class="card-body">
      <div class="form-row">
        <div class="form-group">
          <label class="form-label" for="setting-upload-conflict">Upload Collision Policy</label>
          <select id="setting-upload-conflict" class="form-select" bind:value={uploadConflict}>
            <option value="Ask">Ask (Prompt on collision)</option>
            <option value="Skip">Skip (Keep remote file)</option>
            <option value="Rename">Rename (Create duplicate)</option>
            <option value="Overwrite">Overwrite (Replace remote file)</option>
          </select>
          <div class="form-hint">Action taken when an uploaded file already exists on the remote FTP host.</div>
        </div>

        <div class="form-group">
          <label class="form-label" for="setting-download-conflict">Download Collision Policy</label>
          <select id="setting-download-conflict" class="form-select" bind:value={downloadConflict}>
            <option value="Ask">Ask (Prompt on collision)</option>
            <option value="Skip">Skip (Keep existing file)</option>
            <option value="Rename">Rename (Create duplicate)</option>
            <option value="Overwrite">Overwrite (Replace existing file)</option>
          </select>
          <div class="form-hint">Action taken when a downloaded file already exists on your local machine.</div>
        </div>
      </div>

      <div class="form-row">
        <div class="form-group" style="margin-bottom: 0;">
          <label class="form-label" for="setting-max-concurrent">Maximum Concurrent Transfers</label>
          <select id="setting-max-concurrent" class="form-select" bind:value={maxConcurrent}>
            {#each [1, 2, 3, 4, 5] as n}
              <option value={n}>{n} {n === 2 ? '(Recommended)' : 'streams'}</option>
            {/each}
          </select>
          <div class="form-hint">Parallel data stream ceiling across File Explorer transfers and deployment uploads.</div>
        </div>

        <div class="form-group" style="margin-bottom: 0;">
          <label class="form-label" for="setting-retention">Deployment History Retention</label>
          <select id="setting-retention" class="form-select" aria-label="Log retention" bind:value={retention}>
            <option value="all">Keep all records (Recommended)</option>
            <option value="90">90 days</option>
            <option value="30">30 days</option>
          </select>
          <div class="form-hint">Audit logs and journaled file records older than this duration will be cleared.</div>
        </div>
      </div>
    </div>
  </div>

  <!-- Section 3: System Status & Information -->
  <div class="card mb-4">
    <div class="card-header">
      <div class="card-title-group">
        <Icon name="info" size={18} />
        <div>
          <h2>System Information</h2>
          <p class="text-sm text-muted">Application runtime and engine status.</p>
        </div>
      </div>
    </div>
    <div class="card-body">
      <div class="settings-about-row">
        <div style="display: flex; align-items: center; gap: 12px;">
          <img src={appIcon} alt="QuickFTP Logo" style="width: 34px; height: 34px; border-radius: 6px; object-fit: contain;" />
          <div>
            <strong>QuickFTP Desktop</strong>
            <p class="text-sm text-muted">Version 0.1.0 · Wails v2 + Go 1.26.5 &amp; Svelte</p>
          </div>
        </div>
        <span class="ready-indicator"><i></i> Operational</span>
      </div>
    </div>
  </div>

  <div class="settings-footer-actions">
    <button class="btn btn-primary" on:click={save} disabled={saving}>
      <Icon name="check" size={16} />
      <span>{saving ? 'Saving...' : 'Save All Preferences'}</span>
    </button>
  </div>
  </div>
</div>

<style>
  .settings-container {
    max-width: 900px;
    width: 100%;
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding-right: 8px;
  }
  .card-title-group {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .card-title-group h2 {
    font-size: 15px;
    font-weight: 650;
    margin: 0;
  }
  .card-title-group p {
    margin: 2px 0 0 0;
  }
  .settings-about-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .settings-about-row strong {
    font-size: 14px;
    font-weight: 600;
  }
  .settings-about-row p {
    margin: 3px 0 0 0;
  }
  .settings-footer-actions {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
    padding-bottom: 24px;
  }
</style>
