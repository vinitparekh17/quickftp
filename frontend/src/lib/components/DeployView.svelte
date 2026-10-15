<script lang="ts">
  import { onMount, onDestroy, afterUpdate } from 'svelte';
  import { projects, connections, deployLogs, deployPhase, deployStatus, activeDeploymentId, selectedProjectName, selectedConnectionName, addToast } from '../stores';
  import { getProjects, getConnections, startDeploy, cancelDeploy, rollbackDeployment } from '../wailsjs';

  let selectedProject = '';
  let selectedConnection = '';

  let enableIISOffline = false;
  let iisOfflineHtml = '<!DOCTYPE html><html><head><title>App Offline</title></head><body><h1>Maintenance in Progress</h1><p>The application is currently being updated. Please check back shortly.</p></body></html>';
  let iisShutdownDelay = 1500;

  let consoleEl: HTMLDivElement;
  let rollingBack = false;
  let startedAt = 0;
  let elapsedSeconds = 0;
  $: elapsedText = `${Math.floor(elapsedSeconds / 60)}m ${elapsedSeconds % 60}s`;
  $: latestTransferLine = [...$deployLogs].reverse().find(l => /Uploading |Upload verified |Backup verified |Removing stale/.test(l.line))?.line ?? 'Waiting for the first operation';
  $: transferProgress = (() => {
    const match = latestTransferLine.match(/\((\d+)\/(\d+),/);
    return match ? { done: Number(match[1]), total: Number(match[2]) } : null;
  })();
  $: byteProgress = (() => {
    const match = latestTransferLine.match(/\((\d+)\/(\d+),\s*(\d+)\/(\d+) bytes\)/);
    return match ? `${match[3]} / ${match[4]} bytes` : '—';
  })();

  const phases = [
    { key: 'build', label: 'Build / Publish' },
    { key: 'planning', label: 'Plan artifacts' },
    { key: 'discover', label: 'Discover remote files' },
    { key: 'backup', label: 'Back up remote files' },
    { key: 'verify_backup', label: 'Verify backups' },
    { key: 'upload', label: 'Upload selected files' },
    { key: 'verify_upload', label: 'Verify uploads' },
    { key: 'delete', label: 'Delete stale files' },
    { key: 'complete', label: 'Complete' },
  ];

  function phaseState(key: string) {
    const current = phases.findIndex(p => p.key === $deployPhase);
    const target = phases.findIndex(p => p.key === key);
    if ($deployStatus === 'completed') return 'completed';
    if ($deployStatus === 'failed' && key === $deployPhase) return 'failed';
    if ($deployStatus === 'cancelled' && key === $deployPhase) return 'cancelled';
    if (current >= 0 && target < current) return 'completed';
    if ($deployStatus === 'running' && key === $deployPhase) return 'running';
    return 'pending';
  }

  onMount(async () => {
    if ($projects.length === 0) {
      $projects = await getProjects();
    }
    if ($connections.length === 0) {
      $connections = await getConnections();
    }
    if ($projects.length > 0 && !selectedProject) {
      selectedProject = $projects.find(p => p.name === $selectedProjectName)?.name ?? $projects[0].name;
    }
    if ($connections.length > 0 && !selectedConnection) {
      selectedConnection = $connections.find(c => c.name === $selectedConnectionName)?.name ?? $connections[0].name;
    }
  });

  afterUpdate(() => {
    if (consoleEl) {
      consoleEl.scrollTop = consoleEl.scrollHeight;
    }
  });

  async function handleStartDeploy() {
    if (!selectedProject) {
      addToast('error', 'Please select a project to deploy');
      return;
    }
    if (!selectedConnection) {
      addToast('error', 'Please select a target FTP connection');
      return;
    }

    try {
      startedAt = Date.now(); elapsedSeconds = 0;
      await startDeploy(selectedProject, selectedConnection, {
        iisOfflinePage: enableIISOffline ? iisOfflineHtml : '',
        iisShutdownDelay: Number(iisShutdownDelay) || 1500,
      });
      addToast('info', `Deploying ${selectedProject} to ${selectedConnection}...`);
    } catch (err: any) {
      addToast('error', `Deployment failed: ${err.message}`);
    }
  }

  async function handleRollback() {
    if (!$activeDeploymentId) return;
    if (!confirm(`Are you sure you want to roll back deployment #${$activeDeploymentId}? This will restore backed up remote files.`)) {
      return;
    }
    rollingBack = true;
    try {
      await rollbackDeployment($activeDeploymentId, selectedConnection);
      addToast('success', `Deployment #${$activeDeploymentId} successfully rolled back`);
      $deployStatus = 'idle';
      $deployPhase = 'rolled_back';
    } catch (err: any) {
      addToast('error', `Rollback failed: ${err.message}`);
    } finally {
      rollingBack = false;
    }
  }

  async function handleCancel() {
    try { await cancelDeploy(); addToast('info', 'Cancellation requested. Waiting for the active operation to stop safely.'); }
    catch (err: any) { addToast('error', `Could not cancel deployment: ${err.message}`); }
  }

  function clearLogs() {
    $deployLogs = [];
  }

  const elapsedTimer = setInterval(() => {
    if ($deployStatus === 'running' && startedAt) elapsedSeconds = Math.floor((Date.now() - startedAt) / 1000);
  }, 1000);
  onDestroy(() => clearInterval(elapsedTimer));
</script>

<section class="page-heading"><div><p class="eyebrow">DEPLOYMENT</p><h1>Deploy project</h1><p class="page-subtitle">Run a verified build and transfer with a recoverable remote backup.</p></div></section>

<div class="card mb-4">
  <div class="card-header">
    <div>
      <h2>New Deployment</h2>
      <p class="text-sm text-muted">Build locally, backup remote tree, stage upload, verify SHA-256, and promote safely</p>
    </div>
    {#if $deployStatus === 'running'}
      <span class="badge badge-warning">Deploying...</span>
    {:else if $deployStatus === 'completed'}
      <span class="badge badge-success">Deployment Complete</span>
    {:else if $deployStatus === 'failed'}
      <span class="badge badge-danger">Deployment Failed</span>
    {:else if $deployStatus === 'cancelled'}
      <span class="badge badge-warning">Deployment Cancelled</span>
    {/if}
  </div>

  <div class="card-body">
    <div class="form-row">
      <div class="form-group">
        <label class="form-label" for="deploy-proj">Select Project</label>
        <select
          id="deploy-proj"
          class="form-select"
          bind:value={selectedProject}
          disabled={$deployStatus === 'running'}
        >
          {#each $projects as p}
            <option value={p.name}>{p.name} ({p.projectType})</option>
          {/each}
        </select>
      </div>

      <div class="form-group">
        <label class="form-label" for="deploy-conn">Target Connection</label>
        <select
          id="deploy-conn"
          class="form-select"
          bind:value={selectedConnection}
          disabled={$deployStatus === 'running'}
        >
          {#each $connections as c}
            <option value={c.name}>{c.name} ({c.protocol === 'ftps_explicit' ? 'FTPS' : 'FTP'} • {c.host})</option>
          {/each}
        </select>
      </div>
    </div>

    <!-- IIS Offline handling toggle -->
    <div class="form-group" style="margin-top: 8px;">
      <label class="form-check">
        <input
          type="checkbox"
          bind:checked={enableIISOffline}
          disabled={$deployStatus === 'running'}
        />
        <span>Enable IIS <code>app_offline.htm</code> handling (for ASP.NET Core on IIS)</span>
      </label>
    </div>

    {#if enableIISOffline}
      <div style="background: var(--color-input-bg); border: 1px solid var(--color-border); border-radius: var(--radius-sm); padding: 12px; margin-bottom: 16px;">
        <div class="form-group">
          <label class="form-label" for="iis-delay">IIS Shutdown Delay (milliseconds)</label>
          <input
            id="iis-delay"
            class="form-input"
            type="number"
            style="max-width: 200px;"
            bind:value={iisShutdownDelay}
            disabled={$deployStatus === 'running'}
          />
          <div class="form-hint">Time allowed for IIS application pool to gracefully release file locks</div>
        </div>
        <div class="form-group" style="margin-bottom: 0;">
          <label class="form-label" for="iis-html">app_offline.htm Content</label>
          <textarea
            id="iis-html"
            class="form-textarea"
            rows="3"
            bind:value={iisOfflineHtml}
            disabled={$deployStatus === 'running'}
          ></textarea>
        </div>
      </div>
    {/if}

    <div style="display: flex; gap: 12px; align-items: center; margin-top: 16px;">
      <button
        class="btn btn-primary"
        on:click={handleStartDeploy}
        disabled={$deployStatus === 'running' || !$projects.length || !$connections.length}
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polygon points="5 3 19 12 5 21 5 3" />
        </svg>
        <span>{$deployStatus === 'running' ? 'Deploying...' : 'Start Safe Deployment'}</span>
      </button>

      {#if $deployStatus === 'running'}
        <button class="btn btn-secondary" on:click={handleCancel}>Cancel deployment</button>
        <span class="text-sm text-muted">Cancellation takes effect after the active FTP operation ends.</span>
      {/if}

      {#if ($deployStatus === 'failed' || $deployStatus === 'cancelled') && $activeDeploymentId}
        <button
          class="btn btn-danger"
          on:click={handleRollback}
          disabled={rollingBack}
        >
          <span>{rollingBack ? 'Rolling back...' : `Rollback Deployment #${$activeDeploymentId}`}</span>
        </button>
      {/if}

      {#if $deployLogs.length > 0}
        <button class="btn btn-ghost btn-sm" on:click={clearLogs} style="margin-left: auto;">
          Clear Console
        </button>
      {/if}
    </div>
  </div>
</div>

<!-- Deployment Status & Phases -->
{#if $deployStatus !== 'idle'}
  <div class="deploy-detail-grid">
  <div class="card mb-4">
    <div class="card-header">
      <h2>Execution Pipeline</h2>
      {#if $deployPhase}
        <span class="badge badge-info">Phase: {$deployPhase}</span>
      {/if}
    </div>
    <div class="card-body">
      <div class="deployment-step-list">
        {#each phases as p}
          <div class="deployment-step" class:step-running={phaseState(p.key) === 'running'} class:step-complete={phaseState(p.key) === 'completed'} class:step-failed={phaseState(p.key) === 'failed'} class:step-cancelled={phaseState(p.key) === 'cancelled'}>
            <span class="step-mark">{phaseState(p.key) === 'completed' ? '✓' : phaseState(p.key) === 'failed' || phaseState(p.key) === 'cancelled' ? '!' : phaseState(p.key) === 'running' ? '•' : '○'}</span><span>{p.label}</span><small>{phaseState(p.key)}</small>
          </div>
        {/each}
      </div>
      <div class="deploy-current"><span>Current operation</span><strong>{latestTransferLine}</strong><div class="deploy-metrics"><span>Elapsed <b>{elapsedText}</b></span><span>Files <b>{transferProgress ? `${transferProgress.done}/${transferProgress.total}` : '—'}</b></span><span>Bytes <b>{byteProgress}</b></span></div></div>
    </div>
  </div>

<!-- Build & Transfer Logs Terminal -->
<div class="card">
  <div class="card-header">
    <h2>Output Console</h2>
    <span class="text-sm text-muted">{$deployLogs.length} events logged</span>
  </div>
  <div class="card-body" style="padding: 0;">
    <div class="console" bind:this={consoleEl} style="min-height: 250px;">
      {#if $deployLogs.length === 0}
        <div class="console-line text-muted">// Logs from build process, FTP staging, and checksum verification will stream here...</div>
      {:else}
        {#each $deployLogs as log}
          <div class="console-line {log.stream}">
            {log.line}
          </div>
        {/each}
      {/if}
    </div>
  </div>
</div>
  </div>
{/if}

