<script lang="ts">
  import { afterUpdate, createEventDispatcher } from 'svelte';
  import Icon from '../Icon.svelte';
  import Pager from '../Pager.svelte';
  import { pageItems } from '../../activityGroups';
  import { getFileIcon, getProjectIcon } from '../../icons';
  import type {
    DeploymentDTO,
    DeploymentFileDTO,
    ProjectDTO,
    ConnectionDTO,
    DeployLog,
  } from '../../types';
  import {
    DEPLOYMENT_STAGES,
    resolveDeploymentStatus,
    resolveStageState,
    isRollbackPossible,
    formatDateTime,
    formatDuration,
    formatBytes,
    formatChecksum,
  } from '../../deploymentUtils';
  import {
    deploymentHistory,
    deployLogs,
    deployPhase,
    deployStatus,
    activeDeploymentId,
    addToast,
  } from '../../stores';
  import {
    getDeploymentFiles,
    getDeploymentLogs,
    clearDeploymentLogs,
    rollbackDeployment,
    cancelDeploy,
  } from '../../wailsjs';

  export let deployment: DeploymentDTO;
  export let allDeployments: DeploymentDTO[] = [];
  export let projects: ProjectDTO[] = [];
  export let connections: ConnectionDTO[] = [];
  export let activeRollbackId: number | null = null;
  export let failedRollbackIds: Set<number> = new Set();

  const dispatch = createEventDispatcher<{
    back: void;
    newDeployment: { projectName: string; connectionName: string };
    refresh: void;
  }>();

  let files: DeploymentFileDTO[] = [];
  let loadingFiles = false;
  let filesLoaded = false;
  let journalOpen = false;
  let filePage = 0;
  let fileFilter: 'all' | 'upload' | 'backup' | 'delete' = 'all';
  let fileSearch = '';

  let consoleEl: HTMLDivElement;
  let activeStepFilter: string | null = null;
  let autoScroll = true;

  // Phase duration tracking during live run
  let phaseTimers: Record<
    string,
    { start?: number; end?: number; duration?: number }
  > = {};

  $: isCurrentRun = ($activeDeploymentId ?? 0) === deployment.id;

  $: isActiveRun =
    $deployStatus === 'running' && $activeDeploymentId === deployment.id;

  $: status = resolveDeploymentStatus(
    deployment,
    $activeDeploymentId,
    $deployStatus === 'running',
    activeRollbackId,
    failedRollbackIds
  );

  $: canRestore = isRollbackPossible(
    deployment,
    allDeployments.length > 0 ? allDeployments : $deploymentHistory
  );

  $: projName =
    deployment.projectName ||
    projects.find((p) => p.id === deployment.projectId)?.name ||
    `Project #${deployment.projectId}`;

  $: projType =
    deployment.projectType ||
    projects.find((p) => p.id === deployment.projectId)?.projectType ||
    'generic';

  $: connName =
    deployment.connectionName ||
    connections.find((c) => c.id === deployment.connectionId)?.name ||
    `Connection #${deployment.connectionId}`;

  $: connHost =
    deployment.connectionHost ||
    connections.find((c) => c.id === deployment.connectionId)?.host ||
    '';

  $: buildCommand =
    deployment.buildCommand ||
    projects.find((p) => p.id === deployment.projectId)?.buildCommand ||
    '';

  $: outputRelPath =
    deployment.outputRelPath ||
    projects.find((p) => p.id === deployment.projectId)?.outputRelPath ||
    '.';

  $: remoteRoot =
    deployment.remoteRoot ||
    connections.find((c) => c.id === deployment.connectionId)?.remoteRoot ||
    '/';

  // Live progress line calculation
  $: latestTransferLine = isActiveRun
    ? ([...$deployLogs]
        .reverse()
        .find((l) =>
          /Uploading |Upload verified |Backup verified |Removing stale/.test(
            l.line
          )
        )?.line ?? '')
    : '';

  $: transferProgress = (() => {
    const match = latestTransferLine.match(/\((\d+)\/(\d+),/);
    return match ? { done: Number(match[1]), total: Number(match[2]) } : null;
  })();

  $: byteProgress = (() => {
    const match = latestTransferLine.match(
      /\((\d+)\/(\d+),\s*(\d+)\/(\d+) bytes\)/
    );
    return match
      ? `${formatBytes(Number(match[3]))} / ${formatBytes(Number(match[4]))}`
      : null;
  })();

  let storedLogs: DeployLog[] = [];
  let logPage = 0;
  let logTotal = 0;
  let logKey = '';
  let logGeneration = 0;
  let loadingLogs = false;
  $: if (!isCurrentRun && deployment.id > 0) {
    const key = `${deployment.id}:${logPage}`;
    if (key !== logKey) {
      logKey = key;
      void loadLogs();
    }
  }
  async function loadLogs() {
    const generation = ++logGeneration;
    loadingLogs = true;
    try {
      const result = await getDeploymentLogs(deployment.id, logPage * 50, 50);
      if (generation === logGeneration) {
        storedLogs = result.entries;
        logTotal = result.total;
      }
    } catch (error: any) {
      if (generation === logGeneration)
        addToast('error', `Could not load run logs: ${error.message}`);
    } finally {
      if (generation === logGeneration) loadingLogs = false;
    }
  }
  async function clearLogs() {
    try {
      if (deployment.id > 0) await clearDeploymentLogs(deployment.id);
      if (isCurrentRun) deployLogs.set([]);
      storedLogs = [];
      logTotal = 0;
      logPage = 0;
      addToast(
        'success',
        'Text logs cleared; deployment journal and backups kept'
      );
    } catch (error: any) {
      addToast('error', `Could not clear logs: ${error.message}`);
    }
  }
  $: runLogs = isCurrentRun ? $deployLogs : storedLogs;
  // Filter logs for console
  $: filteredLogs = (() => {
    if (!activeStepFilter) return runLogs;
    const filterLower = activeStepFilter.toLowerCase();
    return runLogs.filter((log) => {
      const l = log.line.toLowerCase();
      if (filterLower === 'build')
        return l.includes('build') || l.includes('dotnet') || l.includes('ng ');
      if (filterLower === 'backup' || filterLower === 'verify_backup')
        return l.includes('backup');
      if (filterLower === 'upload' || filterLower === 'verify_upload')
        return l.includes('upload') || l.includes('stor');
      if (filterLower === 'delete')
        return l.includes('stale') || l.includes('remov') || l.includes('dele');
      return l.includes(filterLower);
    });
  })();

  // Filter files in artifact operations journal
  $: filteredFiles = files.filter((f) => {
    if (fileFilter !== 'all' && f.stage !== fileFilter) return false;
    if (fileSearch.trim()) {
      const q = fileSearch.trim().toLowerCase();
      return (
        f.relativePath.toLowerCase().includes(q) ||
        f.status.toLowerCase().includes(q) ||
        f.errorDetail?.toLowerCase().includes(q) ||
        (f.sha256Local && f.sha256Local.toLowerCase().includes(q))
      );
    }
    return true;
  });

  $: filePage = Math.min(
    filePage,
    Math.max(0, Math.ceil(filteredFiles.length / 50) - 1)
  );

  $: uploadCount = files.filter((f) => f.stage === 'upload').length;
  $: backupCount = files.filter((f) => f.stage === 'backup').length;
  $: deleteCount = files.filter((f) => f.stage === 'delete').length;
  $: totalUploadBytes = files
    .filter((f) => f.stage === 'upload')
    .reduce((sum, f) => sum + (f.sizeBytes || 0), 0);

  let previousRunStatus = deployment.status;
  $: if (deployment.status !== previousRunStatus) {
    previousRunStatus = deployment.status;
    if (deployment.status !== 'running') {
      filesLoaded = false;
      if (journalOpen) void loadFiles();
    }
  }

  afterUpdate(() => {
    if (consoleEl && autoScroll && isActiveRun) {
      consoleEl.scrollTop = consoleEl.scrollHeight;
    }
  });

  async function loadFiles() {
    if (deployment.id === 0) {
      files = [];
      loadingFiles = false;
      return;
    }
    loadingFiles = true;
    try {
      // Fetch all files across all stages for this deployment
      files = await getDeploymentFiles(deployment.id, '');
      filesLoaded = true;
    } catch (err: any) {
      addToast('error', `Failed to load file journal: ${err.message}`);
    } finally {
      loadingFiles = false;
    }
  }

  async function handleRollback() {
    if (
      !confirm(
        `Roll back deployment #${deployment.id} on "${connName}"?\n\nThis will restore all modified and deleted remote files using the verified local snapshot in:\n${deployment.backupDir}`
      )
    ) {
      return;
    }

    activeRollbackId = deployment.id;
    try {
      await rollbackDeployment(deployment.id, connName);
      addToast(
        'success',
        `Deployment #${deployment.id} successfully rolled back`
      );
      dispatch('refresh');
    } catch (err: any) {
      failedRollbackIds.add(deployment.id);
      failedRollbackIds = new Set(failedRollbackIds);
      addToast('error', `Rollback failed: ${err.message}`);
    } finally {
      activeRollbackId = null;
    }
  }

  async function handleCancel() {
    try {
      await cancelDeploy();
      addToast(
        'info',
        'Cancellation requested. Active FTP operation will safely terminate.'
      );
    } catch (err: any) {
      addToast('error', `Could not cancel deployment: ${err.message}`);
    }
  }

  async function copyBackupDir() {
    if (!deployment.backupDir) return;
    try {
      await navigator.clipboard.writeText(deployment.backupDir);
      addToast('success', 'Backup directory copied to clipboard');
    } catch {
      addToast('info', deployment.backupDir);
    }
  }

  async function copyLogs() {
    const text = filteredLogs.map((l) => l.line).join('\n');
    if (!text) {
      addToast('info', 'No execution logs available to copy');
      return;
    }
    try {
      await navigator.clipboard.writeText(text);
      addToast('success', 'Execution logs copied to clipboard');
    } catch {
      addToast('error', 'Could not copy logs');
    }
  }
</script>

<div class="view-full-height">
  <!-- Detail Top Header Bar with Breadcrumbs & Action Bar -->
  <section class="detail-header-card">
    <div class="detail-nav-bar">
      <button class="btn btn-ghost btn-sm" on:click={() => dispatch('back')}>
        <Icon name="back" size={14} />
        <span>Deployments</span>
      </button>
      <span class="nav-sep">/</span>
      <span class="nav-current text-mono"
        >{deployment.id > 0 ? `Run #${deployment.id}` : 'New run'}</span
      >
    </div>

    <div class="detail-title-row">
      <div class="title-left">
        <span class="badge {status.badgeClass} status-badge-large">
          <Icon name={status.icon} size={14} spin={status.spin} />
          <span>{status.label}</span>
        </span>
        <div class="title-text-group">
          <h2>
            {deployment.id > 0
              ? `Deployment #${deployment.id}`
              : 'New Deployment'}
          </h2>
          <div class="title-meta text-muted">
            <span class="meta-item">
              <Icon name={getProjectIcon(projType)} size={14} />
              <strong>{projName}</strong>
            </span>
            <span class="meta-sep">→</span>
            <span class="meta-item">
              <Icon name="server" size={14} />
              <span>{connName}</span>
              {#if connHost}
                <span class="text-mono text-xs">({connHost})</span>
              {/if}
            </span>
          </div>
        </div>
      </div>

      <!-- Action Buttons -->
      <div class="detail-actions">
        <!-- New Deployment Using Same Config -->
        {#if !isActiveRun}
          <button
            class="btn btn-secondary btn-sm"
            on:click={() =>
              dispatch('newDeployment', {
                projectName: projName,
                connectionName: connName,
              })}
            disabled={$deployStatus === 'running'}
            title="Start a new deployment attempt with this configuration"
          >
            <Icon name="play" size={14} />
            <span>Deploy Again</span>
          </button>
        {/if}

        <!-- Cancel (if running) -->
        {#if isActiveRun}
          <button
            class="btn btn-secondary btn-sm text-danger"
            on:click={handleCancel}
          >
            <Icon name="close" size={14} />
            <span>Cancel Run</span>
          </button>
        {/if}

        <!-- Rollback button: only visible if this run is the most recent failed deployment -->
        {#if canRestore}
          <button
            class="btn btn-danger btn-sm"
            on:click={handleRollback}
            disabled={activeRollbackId === deployment.id}
            title="Restore remote files from backup snapshot"
          >
            <Icon
              name="rollback"
              size={14}
              spin={activeRollbackId === deployment.id}
            />
            <span
              >{activeRollbackId === deployment.id
                ? 'Rolling back...'
                : 'Roll Back Remote'}</span
            >
          </button>
        {/if}
      </div>
    </div>

    <!-- Configuration Snapshot Metadata Bar -->
    <details class="run-configuration">
      <summary>Configuration &amp; backup details</summary>
      <div class="snapshot-grid">
        <div class="snapshot-item">
          <span class="snapshot-k">Started</span>
          <span class="snapshot-v">{formatDateTime(deployment.startedAt)}</span>
        </div>
        <div class="snapshot-item">
          <span class="snapshot-k">Finished</span>
          <span class="snapshot-v">
            {deployment.finishedAt
              ? formatDateTime(deployment.finishedAt)
              : isActiveRun
                ? 'In progress...'
                : 'Interrupted'}
          </span>
        </div>
        <div class="snapshot-item">
          <span class="snapshot-k">Duration</span>
          <span class="snapshot-v text-mono">
            {formatDuration(deployment.startedAt, deployment.finishedAt)}
          </span>
        </div>
        <div class="snapshot-item">
          <span class="snapshot-k">Build Command</span>
          <span class="snapshot-v text-mono text-xs">{buildCommand || '—'}</span
          >
        </div>
        <div class="snapshot-item">
          <span class="snapshot-k">Output Path</span>
          <span class="snapshot-v text-mono text-xs">{outputRelPath}</span>
        </div>
        <div class="snapshot-item">
          <span class="snapshot-k">Remote Root</span>
          <span class="snapshot-v text-mono text-xs">{remoteRoot}</span>
        </div>
        {#if deployment.backupDir}
          <div class="snapshot-item snapshot-col-span">
            <span class="snapshot-k">Backup Snapshot Directory</span>
            <div class="backup-path-row">
              <span class="snapshot-v text-mono text-xs backup-path-text"
                >{deployment.backupDir}</span
              >
              <button
                class="table-icon-action action-secondary"
                on:click={copyBackupDir}
                title="Copy backup path"
                aria-label="Copy backup directory path"
              >
                <Icon name="copy" size={12} />
              </button>
            </div>
          </div>
        {/if}
      </div>
    </details>

    <!-- Error Summary Banner (when failure or cancellation occurred) -->
    {#if deployment.errorMessage || status.key === 'rollback_failed'}
      <div class="run-error-banner">
        <div class="error-banner-icon">
          <Icon name="error" size={18} />
        </div>
        <div class="error-banner-content">
          <div class="error-title">
            {#if status.key === 'rollback_failed'}
              Rollback Operation Failed
            {:else if status.key === 'cancelled'}
              Deployment Cancelled
            {:else}
              Deployment Failed in phase: <code>{deployment.currentPhase}</code>
            {/if}
          </div>
          <div class="error-desc text-mono text-xs">
            {deployment.errorMessage ||
              'Unknown error occurred during execution'}
          </div>
          {#if deployment.backupDir && status.key !== 'rolled_back'}
            {#if canRestore}
              <div class="error-advice text-xs mt-1">
                Your remote files from before this run remain safely backed up
                in the snapshot directory above. You can trigger an explicit
                rollback to restore them.
              </div>
            {:else}
              <div class="error-advice text-xs mt-1 text-muted">
                Restore is disabled for this run because subsequent deployments
                have already altered the remote target.
              </div>
            {/if}
          {/if}
        </div>
      </div>
    {/if}
  </section>

  <!-- Main Body: Pipeline Execution Stages + Console Terminal -->
  <div class="view-scrollable">
    <div class="cockpit-layout-grid">
      <!-- Left Column: Pipeline Stages Stepper -->
      <div class="card stages-card">
        <div class="card-header stages-header">
          <div class="stages-header-title">
            <Icon name="deploy" size={15} />
            <span>Execution Stages</span>
          </div>
          <span class="text-xs text-muted">
            {#if status.key === 'completed'}
              All stages verified
            {:else if isActiveRun}
              Phase: <b>{$deployPhase || 'executing'}</b>
            {:else}
              Status: <b>{status.label}</b>
            {/if}
          </span>
        </div>

        <div class="stages-list">
          {#each DEPLOYMENT_STAGES as stage, idx}
            {@const stageState = resolveStageState(
              stage.key,
              isActiveRun ? $deployPhase : deployment.currentPhase,
              status.key
            )}

            {#if stageState !== 'skipped'}
              <button
                type="button"
                class="stage-row"
                class:is-active={stageState === 'running'}
                class:is-completed={stageState === 'completed'}
                class:is-failed={stageState === 'failed'}
                class:is-cancelled={stageState === 'cancelled'}
                class:is-selected={activeStepFilter === stage.key}
                on:click={() =>
                  (activeStepFilter =
                    activeStepFilter === stage.key ? null : stage.key)}
                title={isCurrentRun
                  ? 'Filter console logs to this stage'
                  : 'Show stage description'}
              >
                <!-- Stage State Icon -->
                <div class="stage-state-icon">
                  {#if stageState === 'completed'}
                    <span class="state-icon-success"
                      ><Icon name="success" size={15} /></span
                    >
                  {:else if stageState === 'failed'}
                    <span class="state-icon-failure"
                      ><Icon name="error" size={15} /></span
                    >
                  {:else if stageState === 'cancelled'}
                    <span class="state-icon-cancelled"
                      ><Icon name="warning" size={15} /></span
                    >
                  {:else if stageState === 'running'}
                    <span class="state-icon-running"
                      ><Icon name="loader" size={15} spin={true} /></span
                    >
                  {:else}
                    <span class="state-icon-pending"
                      ><Icon name="clock" size={14} /></span
                    >
                  {/if}
                </div>

                <!-- Stage Body -->
                <div class="stage-body">
                  <div class="stage-title-row">
                    <span class="stage-num">{idx + 1}.</span>
                    <span class="stage-label">{stage.label}</span>
                    {#if stage.isDestructive}
                      <span
                        class="destructive-tag"
                        title="Destructive remote cleanup operation">prune</span
                      >
                    {/if}
                  </div>
                  {#if activeStepFilter === stage.key}<div
                      class="stage-desc text-muted"
                    >
                      {stage.description}
                    </div>{/if}
                </div>

                <!-- Stage Chevron -->
                <div class="stage-arrow">
                  <Icon
                    name={activeStepFilter === stage.key
                      ? 'chevronDown'
                      : 'chevronRight'}
                    size={12}
                  />
                </div>
              </button>
            {/if}
          {/each}
        </div>

        <!-- Live Transfer Footer Indicator -->
        {#if isActiveRun && latestTransferLine}
          <div class="live-progress-bar">
            <span class="live-pulse-dot"></span>
            <div class="live-progress-text">
              <span class="live-progress-label">Active Transfer</span>
              <span
                class="live-progress-desc text-mono"
                title={latestTransferLine}
              >
                {latestTransferLine}
              </span>
              {#if transferProgress}
                <div class="live-counts text-xs text-muted mt-1">
                  Files: <b
                    >{transferProgress.done} / {transferProgress.total}</b
                  >
                  {#if byteProgress}
                    • {byteProgress}
                  {/if}
                </div>
              {/if}
            </div>
          </div>
        {/if}
      </div>

      <!-- Right Column: Execution Terminal / Console -->
      <div class="terminal-card">
        <div class="terminal-header">
          <div class="terminal-title">
            <Icon name="fileCode" size={14} />
            <span>
              {#if activeStepFilter}
                Filter: <b
                  >{DEPLOYMENT_STAGES.find((s) => s.key === activeStepFilter)
                    ?.label || activeStepFilter}</b
                >
              {:else if isActiveRun}
                Live Execution Console
              {:else}
                Execution Output
              {/if}
            </span>
          </div>

          <div class="terminal-controls">
            {#if activeStepFilter}
              <button
                class="btn btn-ghost btn-xs terminal-clear-filter"
                on:click={() => (activeStepFilter = null)}
              >
                <Icon name="close" size={11} />
                <span>Clear filter</span>
              </button>
            {/if}

            {#if isCurrentRun}
              <span class="text-xs text-muted terminal-count">
                {filteredLogs.length} lines
              </span>
              <label class="terminal-autoscroll text-xs">
                <input type="checkbox" bind:checked={autoScroll} />
                <span>Auto-scroll</span>
              </label>
            {/if}

            {#if filteredLogs.length > 0 || logTotal > 0}<button
                class="btn btn-ghost btn-xs text-muted"
                on:click={clearLogs}>Clear logs</button
              >{/if}
            {#if filteredLogs.length > 0}
              <button
                class="btn btn-ghost btn-xs text-muted"
                on:click={copyLogs}
                title="Copy terminal logs"
              >
                <Icon name="copy" size={12} />
              </button>
            {/if}
          </div>
        </div>

        <!-- Console Body -->
        <div class="terminal-body" bind:this={consoleEl}>
          {#if loadingLogs}<div class="terminal-empty">
              Loading this deployment’s saved logs…
            </div>
          {:else if isCurrentRun || logTotal > 0}
            {#if filteredLogs.length === 0}
              <div class="terminal-empty text-muted">
                <Icon name="clock" size={24} />
                <span>
                  {#if activeStepFilter}
                    No log output recorded for stage "{activeStepFilter}".
                  {:else}
                    Awaiting compiler output and remote file transfer stream...
                  {/if}
                </span>
              </div>
            {:else}
              <div class="terminal-lines">
                {#each filteredLogs as log, idx}
                  <div class="terminal-row {log.stream}">
                    <span class="line-num">{idx + 1}</span>
                    <span class="line-text">{log.line}</span>
                  </div>
                {/each}
              </div>
            {/if}
          {:else}
            <!-- Historical run console summary -->
            <div class="historical-console-summary">
              <div class="terminal-row stdout">
                <span class="line-num">1</span>
                <span class="line-text text-success">
                  ✓ Deployment record #{deployment.id} initialized
                </span>
              </div>
              <div class="terminal-row stdout">
                <span class="line-num">2</span>
                <span class="line-text">
                  Project: {projName} ({projType}) → Host: {connName} ({connHost})
                </span>
              </div>
              <div class="terminal-row stdout">
                <span class="line-num">3</span>
                <span class="line-text">
                  Local Backup Path: {deployment.backupDir || 'None'}
                </span>
              </div>
              <div class="terminal-row stdout">
                <span class="line-num">4</span>
                <span class="line-text">
                  Remote Destination: {remoteRoot}
                </span>
              </div>
              {#if deployment.errorMessage}
                <div class="terminal-row stderr">
                  <span class="line-num">5</span>
                  <span class="line-text">
                    [ERROR] {deployment.errorMessage}
                  </span>
                </div>
              {:else if deployment.status === 'completed'}
                <div class="terminal-row success">
                  <span class="line-num">5</span>
                  <span class="line-text text-success">
                    ✓ All remote operations verified and committed to journal.
                  </span>
                </div>
              {/if}
              <div class="terminal-historical-note text-muted text-xs">
                Detailed file-by-file outcomes and SHA-256 verification records
                are preserved in the Artifact Operations Journal below.
              </div>
            </div>
          {/if}
        </div>
        {#if !isCurrentRun}<Pager bind:page={logPage} total={logTotal} />{/if}
      </div>
    </div>

    <!-- Artifact Operations Journal Card -->
    <details
      class="card journal-card mt-3"
      bind:open={journalOpen}
      on:toggle={() => {
        if (journalOpen && !filesLoaded && !loadingFiles) void loadFiles();
      }}
    >
      <summary class="journal-disclosure">
        <span class="journal-header-left">
          <Icon name="archive" size={15} />
          <strong>File operations</strong>
          <span class="journal-metrics-summary text-xs text-muted">
            {#if files.length > 0}
              {uploadCount} uploads ({formatBytes(totalUploadBytes)}) · {backupCount}
              backups · {deleteCount} removed
            {:else}{filesLoaded
                ? 'No operations recorded'
                : 'Open to load this run’s files'}{/if}
          </span>
        </span>
      </summary>
      <div>
        <div class="card-header journal-header">
          <!-- Stage Filter Tabs & Search -->
          <div class="journal-header-controls">
            <div class="journal-tabs">
              <button
                class="journal-tab-btn"
                class:is-active={fileFilter === 'all'}
                on:click={() => {
                  fileFilter = 'all';
                  filePage = 0;
                }}
              >
                All ({files.length})
              </button>
              <button
                class="journal-tab-btn"
                class:is-active={fileFilter === 'upload'}
                on:click={() => {
                  fileFilter = 'upload';
                  filePage = 0;
                }}
              >
                Uploads ({uploadCount})
              </button>
              <button
                class="journal-tab-btn"
                class:is-active={fileFilter === 'backup'}
                on:click={() => {
                  fileFilter = 'backup';
                  filePage = 0;
                }}
              >
                Backups ({backupCount})
              </button>
              {#if deleteCount > 0}
                <button
                  class="journal-tab-btn"
                  class:is-active={fileFilter === 'delete'}
                  on:click={() => {
                    fileFilter = 'delete';
                    filePage = 0;
                  }}
                >
                  Pruned ({deleteCount})
                </button>
              {/if}
            </div>

            <div class="journal-search-wrap">
              <Icon name="search" size={13} />
              <input
                class="form-input form-input-xs journal-search-input"
                placeholder="Search file path or hash..."
                bind:value={fileSearch}
                on:input={() => (filePage = 0)}
              />
            </div>
          </div>
        </div>

        <!-- Journal Table -->
        <div class="card-body" style="padding: 0;">
          {#if loadingFiles}
            <div class="journal-loading text-muted text-sm">
              <Icon name="loader" size={16} spin={true} />
              <span>Loading artifact journal...</span>
            </div>
          {:else if files.length === 0}
            <div class="journal-empty text-muted text-sm">
              No journaled artifact operations recorded for this deployment.
            </div>
          {:else if filteredFiles.length === 0}
            <div class="journal-empty text-muted text-sm">
              No files match the selected filter.
            </div>
          {:else}
            <div class="table-wrap journal-table-wrap">
              <table class="journal-table">
                <thead>
                  <tr>
                    <th style="width: 100px;">Stage</th>
                    <th>Remote Path</th>
                    <th style="width: 100px;">Size</th>
                    <th style="width: 170px;">Verification</th>
                    <th style="width: 150px;">SHA-256 Digest</th>
                    <th>Outcome / Detail</th>
                  </tr>
                </thead>
                <tbody>
                  {#each pageItems(filteredFiles, filePage) as f (f.id)}
                    <tr>
                      <!-- Stage -->
                      <td>
                        <span
                          class="badge text-xs"
                          class:badge-info={f.stage === 'upload'}
                          class:badge-gray={f.stage === 'backup'}
                          class:badge-warning={f.stage === 'delete'}
                        >
                          <Icon
                            name={f.stage === 'upload'
                              ? 'upload'
                              : f.stage === 'backup'
                                ? 'archive'
                                : 'trash'}
                            size={11}
                          />
                          <span>{f.stage}</span>
                        </span>
                      </td>

                      <!-- Remote Path -->
                      <td class="text-mono text-xs">
                        <span class="file-icon mr-1">
                          <Icon
                            name={getFileIcon(f.relativePath, false)}
                            size={13}
                          />
                        </span>
                        <span>{f.relativePath}</span>
                      </td>

                      <!-- Size -->
                      <td class="text-mono text-xs text-muted">
                        {formatBytes(f.sizeBytes)}
                      </td>

                      <!-- Verification Status -->
                      <td>
                        {#if f.status === 'verified'}
                          <span class="badge badge-success text-xs">
                            <Icon name="check" size={11} />
                            <span>SHA-256 verified</span>
                          </span>
                        {:else if f.status === 'completed'}
                          <span class="badge badge-info text-xs">
                            <Icon name="check" size={11} />
                            <span>Size verified</span>
                          </span>
                        {:else if f.status === 'failed'}
                          <span class="badge badge-danger text-xs">
                            <Icon name="error" size={11} />
                            <span>Failed</span>
                          </span>
                        {:else}
                          <span class="badge badge-gray text-xs">
                            <Icon name="clock" size={11} />
                            <span>{f.status}</span>
                          </span>
                        {/if}
                      </td>

                      <!-- SHA-256 Digest -->
                      <td
                        class="text-mono text-xs text-muted"
                        title={f.sha256Local || ''}
                      >
                        {#if f.sha256Local}
                          {formatChecksum(f.sha256Local)}
                        {:else}
                          —
                        {/if}
                      </td>

                      <!-- Outcome / Error Detail -->
                      <td class="text-xs">
                        {#if f.errorDetail}
                          <span class="text-danger font-medium"
                            >{f.errorDetail}</span
                          >
                        {:else if f.status === 'verified' || f.status === 'completed'}
                          <span class="text-muted">Atomic promote complete</span
                          >
                        {:else}
                          <span class="text-muted">—</span>
                        {/if}
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>
        <Pager bind:page={filePage} total={filteredFiles.length} />
      </div>
    </details>
  </div>
</div>

<style>
  /* Header Card */
  .detail-header-card {
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    padding: 16px 20px;
    margin-bottom: 14px;
    box-shadow: var(--shadow-sm);
  }

  .detail-nav-bar {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 12px;
  }

  .nav-sep {
    color: var(--color-text-light);
  }

  .nav-current {
    font-size: 13px;
    font-weight: 600;
    color: var(--color-text);
  }

  .detail-title-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    flex-wrap: wrap;
    gap: 16px;
    margin-bottom: 14px;
  }

  .title-left {
    display: flex;
    align-items: center;
    gap: 14px;
  }

  .status-badge-large {
    font-size: 12.5px;
    padding: 6px 12px;
  }

  .title-text-group h2 {
    margin: 0;
    font-size: 18px;
    font-weight: 700;
  }

  .title-meta {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    margin-top: 2px;
  }

  .meta-item {
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }

  .meta-sep {
    color: var(--color-text-light);
  }

  .detail-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  /* Snapshot Grid */
  .snapshot-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: 10px 16px;
    background: var(--color-input-bg);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    padding: 10px 14px;
  }

  .snapshot-item {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .snapshot-col-span {
    grid-column: 1 / -1;
  }

  .snapshot-k {
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    color: var(--color-text-light);
    letter-spacing: 0.04em;
  }

  .snapshot-v {
    font-size: 12.5px;
    font-weight: 500;
    color: var(--color-text);
  }

  .backup-path-row {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .backup-path-text {
    word-break: break-all;
  }

  /* Error Banner */
  .run-error-banner {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    background: #fef2f2;
    border: 1px solid #fecaca;
    border-radius: var(--radius-sm);
    padding: 12px 14px;
    margin-top: 14px;
    color: var(--color-danger);
  }

  .error-banner-content {
    flex: 1;
  }

  .error-title {
    font-size: 13px;
    font-weight: 700;
  }

  .error-desc {
    margin-top: 3px;
    color: #991b1b;
  }

  .error-advice {
    color: #7f1d1d;
  }

  /* Cockpit Split Grid */
  .cockpit-layout-grid {
    display: grid;
    grid-template-columns: minmax(320px, 370px) minmax(0, 1fr);
    gap: 14px;
    align-items: stretch;
  }

  .stages-card {
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .stages-header {
    background: #f8fafc;
    padding: 10px 14px;
  }

  .stages-header-title {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    font-weight: 600;
  }

  .stages-list {
    display: flex;
    flex-direction: column;
    padding: 4px 0;
  }

  .stage-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 12px;
    background: transparent;
    border: none;
    border-bottom: 1px solid #f1f5f9;
    width: 100%;
    text-align: left;
    cursor: pointer;
    transition: background 0.12s ease;
    font-family: inherit;
  }

  .stage-row:last-child {
    border-bottom: none;
  }

  .stage-row:hover {
    background: #f8fafc;
  }

  .stage-row.is-selected {
    background: #eff6ff;
    border-left: 3px solid var(--color-primary);
  }

  .stage-row.is-active {
    background: #f0f7ff;
  }

  .stage-state-icon {
    width: 20px;
    height: 20px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .state-icon-success {
    color: var(--color-success);
  }

  .state-icon-failure {
    color: var(--color-danger);
  }

  .state-icon-cancelled {
    color: var(--color-warning);
  }

  .state-icon-running {
    color: var(--color-primary);
  }

  .state-icon-pending {
    color: var(--color-text-light);
  }

  .stage-body {
    flex: 1;
    min-width: 0;
  }

  .stage-title-row {
    display: flex;
    align-items: center;
    gap: 5px;
    font-size: 12px;
    font-weight: 600;
  }

  .stage-row.is-active .stage-label {
    color: var(--color-primary);
    font-weight: 700;
  }

  .stage-row.is-failed .stage-label {
    color: var(--color-danger);
    font-weight: 700;
  }

  .stage-num {
    font-size: 10px;
    color: var(--color-text-light);
    min-width: 14px;
  }

  .stage-desc {
    font-size: 10.5px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    margin-top: 1px;
  }

  .destructive-tag {
    font-size: 9px;
    text-transform: uppercase;
    font-weight: 700;
    color: #b45309;
    background: #fef3c7;
    border: 1px solid #fde68a;
    border-radius: 3px;
    padding: 0 4px;
  }

  .stage-arrow {
    color: var(--color-text-light);
  }

  .live-progress-bar {
    margin-top: auto;
    padding: 10px 14px;
    background: #f8fafc;
    border-top: 1px solid var(--color-border);
    display: flex;
    align-items: flex-start;
    gap: 8px;
    font-size: 11px;
  }

  .live-pulse-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #16a34a;
    margin-top: 4px;
    box-shadow: 0 0 0 2px rgba(22, 163, 74, 0.2);
    animation: live-pulse 2s infinite;
  }

  @keyframes live-pulse {
    0% {
      transform: scale(0.95);
      box-shadow: 0 0 0 0 rgba(22, 163, 74, 0.5);
    }
    70% {
      transform: scale(1);
      box-shadow: 0 0 0 5px rgba(22, 163, 74, 0);
    }
    100% {
      transform: scale(0.95);
      box-shadow: 0 0 0 0 rgba(22, 163, 74, 0);
    }
  }

  .live-progress-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .live-progress-label {
    font-size: 9px;
    font-weight: 700;
    text-transform: uppercase;
    color: var(--color-text-secondary);
  }

  .live-progress-desc {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-weight: 600;
    color: var(--color-text);
  }

  /* Terminal Card */
  .terminal-card {
    background: var(--color-console-bg);
    border: 1px solid #334155;
    border-radius: var(--radius);
    display: flex;
    flex-direction: column;
    overflow: hidden;
    min-height: 400px;
    max-height: 560px;
    box-shadow: var(--shadow-md);
  }

  .terminal-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 14px;
    background: #1e293b;
    border-bottom: 1px solid #334155;
    color: #e2e8f0;
    font-size: 12px;
  }

  .terminal-title {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 600;
  }

  .terminal-controls {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .terminal-clear-filter {
    color: #94a3b8;
    background: #334155;
    border-radius: 4px;
    padding: 1px 6px;
  }

  .terminal-autoscroll {
    display: flex;
    align-items: center;
    gap: 5px;
    color: #94a3b8;
    cursor: pointer;
  }

  .terminal-body {
    flex: 1;
    overflow-y: auto;
    font-family: 'Consolas', 'Monaco', ui-monospace, SFMono-Regular, monospace;
    font-size: 11.5px;
    line-height: 1.5;
    padding: 8px 0;
    color: var(--color-console-text);
  }

  .terminal-empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 48px 24px;
    gap: 12px;
    color: #64748b;
    text-align: center;
  }

  .terminal-lines {
    display: flex;
    flex-direction: column;
  }

  .terminal-row {
    display: flex;
    align-items: flex-start;
    padding: 1px 12px;
    word-break: break-all;
  }

  .terminal-row:hover {
    background: rgba(255, 255, 255, 0.04);
  }

  .line-num {
    color: #475569;
    text-align: right;
    min-width: 32px;
    margin-right: 14px;
    font-size: 10px;
    padding-top: 1px;
  }

  .line-text {
    flex: 1;
    color: #e2e8f0;
    white-space: pre-wrap;
  }

  .terminal-row.stderr .line-text {
    color: var(--color-console-red);
  }

  .terminal-row.success .line-text {
    color: var(--color-console-green);
  }

  .historical-console-summary {
    padding: 8px 0;
  }

  .terminal-historical-note {
    padding: 12px 16px;
    color: #64748b;
    border-top: 1px solid #1e293b;
    margin-top: 8px;
  }

  /* Journal Card */
  .journal-card {
    overflow: hidden;
  }

  .journal-header {
    background: #f8fafc;
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 10px 16px;
    flex-wrap: wrap;
    gap: 12px;
  }

  .journal-header-left {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
  }

  .journal-header-controls {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
  }

  .journal-tabs {
    display: flex;
    background: #e2e8f0;
    border-radius: var(--radius-sm);
    padding: 2px;
    gap: 2px;
  }

  .journal-tab-btn {
    border: none;
    background: transparent;
    padding: 3px 10px;
    border-radius: 3px;
    font-size: 11.5px;
    font-weight: 500;
    color: var(--color-text-secondary);
    cursor: pointer;
    transition: all 0.12s;
  }

  .journal-tab-btn:hover {
    color: var(--color-text);
  }

  .journal-tab-btn.is-active {
    background: #ffffff;
    color: var(--color-text);
    font-weight: 600;
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  }

  .journal-search-wrap {
    position: relative;
    display: flex;
    align-items: center;
  }

  .journal-search-wrap :global(.lucide-icon-wrap) {
    position: absolute;
    left: 8px;
    color: var(--color-text-light);
    pointer-events: none;
  }

  .journal-search-input {
    padding-left: 26px;
    padding-top: 3px;
    padding-bottom: 3px;
    font-size: 12px;
    width: 180px;
    background: #ffffff;
  }

  .journal-table-wrap {
    max-height: 280px;
  }

  .journal-table {
    margin: 0;
  }

  .journal-loading,
  .journal-empty {
    padding: 36px 16px;
    text-align: center;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
  }

  @media (max-width: 950px) {
    .cockpit-layout-grid {
      grid-template-columns: 1fr;
    }

    .terminal-card {
      grid-row: 1;
      min-height: 280px;
      max-height: 400px;
    }
    .stages-card {
      grid-row: 2;
    }
    .stages-list {
      display: grid;
      grid-template-columns: 1fr 1fr;
    }
  }
</style>
