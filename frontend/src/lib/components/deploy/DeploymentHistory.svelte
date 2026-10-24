<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import Icon from '../Icon.svelte';
  import { getProjectIcon } from '../../icons';
  import type { DeploymentDTO, ProjectDTO, ConnectionDTO } from '../../types';
  import {
    resolveDeploymentStatus,
    isRollbackPossible,
    formatDateTime,
    formatDuration,
  } from '../../deploymentUtils';
  import {
    deployStatus,
    activeDeploymentId,
    pendingDeployments,
    addToast,
  } from '../../stores';
  import { rollbackDeployment } from '../../wailsjs';

  export let deployments: DeploymentDTO[] = [];
  export let projects: ProjectDTO[] = [];
  export let connections: ConnectionDTO[] = [];
  export let loading = false;
  export let activeRollbackId: number | null = null;
  export let failedRollbackIds: Set<number> = new Set();

  const dispatch = createEventDispatcher<{
    selectRun: number;
    newDeployment: { projectName?: string; connectionName?: string } | void;
    refresh: void;
  }>();

  // Filters
  let searchQuery = '';
  let selectedProjectFilter = 'all';
  let selectedConnectionFilter = 'all';
  let selectedStatusFilter = 'all';

  // Summary counts
  $: totalCount = deployments.length;
  $: successCount = deployments.filter((d) => d.status === 'completed').length;
  $: failedCount = deployments.filter(
    (d) =>
      d.status === 'failed' &&
      !d.cancelled &&
      !d.errorMessage?.includes('context canceled')
  ).length;
  $: activeCount = deployments.filter(
    (d) =>
      d.status === 'running' &&
      ($deployStatus === 'running' || $pendingDeployments.some((p) => p.id === d.id))
  ).length;

  // Filtered deployment rows
  $: filteredDeployments = deployments.filter((d) => {
    const projName =
      d.projectName ||
      projects.find((p) => p.id === d.projectId)?.name ||
      `Project #${d.projectId}`;
    const connName =
      d.connectionName ||
      connections.find((c) => c.id === d.connectionId)?.name ||
      `Connection #${d.connectionId}`;
    const connHost =
      d.connectionHost ||
      connections.find((c) => c.id === d.connectionId)?.host ||
      '';

    // Status matching
    const statusMeta = resolveDeploymentStatus(
      d,
      $activeDeploymentId,
      $deployStatus === 'running',
      activeRollbackId,
      failedRollbackIds
    );

    if (selectedStatusFilter !== 'all') {
      if (selectedStatusFilter === 'success' && statusMeta.key !== 'completed') return false;
      if (selectedStatusFilter === 'failed' && statusMeta.key !== 'failed' && statusMeta.key !== 'rollback_failed') return false;
      if (selectedStatusFilter === 'cancelled' && statusMeta.key !== 'cancelled') return false;
      if (selectedStatusFilter === 'rolled_back' && statusMeta.key !== 'rolled_back') return false;
      if (selectedStatusFilter === 'running' && statusMeta.key !== 'running' && statusMeta.key !== 'rolling_back') return false;
      if (selectedStatusFilter === 'interrupted' && statusMeta.key !== 'interrupted') return false;
    }

    // Project matching
    if (selectedProjectFilter !== 'all' && projName !== selectedProjectFilter) {
      return false;
    }

    // Connection matching
    if (selectedConnectionFilter !== 'all' && connName !== selectedConnectionFilter) {
      return false;
    }

    // Search query matching
    if (searchQuery.trim()) {
      const q = searchQuery.trim().toLowerCase();
      const haystack = `${d.id} ${projName} ${connName} ${connHost} ${statusMeta.label} ${d.currentPhase} ${d.errorMessage}`.toLowerCase();
      if (!haystack.includes(q)) return false;
    }

    return true;
  });

  async function handleRowRollback(d: DeploymentDTO, e: MouseEvent) {
    e.stopPropagation();
    const connName =
      d.connectionName ||
      connections.find((c) => c.id === d.connectionId)?.name ||
      `Connection #${d.connectionId}`;

    if (
      !confirm(
        `Roll back deployment #${d.id} on "${connName}"?\n\nThis will restore remote files to the verified snapshot taken before this deployment began.`
      )
    ) {
      return;
    }

    activeRollbackId = d.id;
    try {
      await rollbackDeployment(d.id, connName);
      addToast('success', `Deployment #${d.id} successfully rolled back`);
      dispatch('refresh');
    } catch (err: any) {
      failedRollbackIds.add(d.id);
      failedRollbackIds = new Set(failedRollbackIds);
      addToast('error', `Rollback failed: ${err.message}`);
    } finally {
      activeRollbackId = null;
    }
  }

  function handleDeployAgain(d: DeploymentDTO, e: MouseEvent) {
    e.stopPropagation();
    const proj =
      d.projectName || projects.find((p) => p.id === d.projectId)?.name || '';
    const conn =
      d.connectionName || connections.find((c) => c.id === d.connectionId)?.name || '';
    dispatch('newDeployment', { projectName: proj, connectionName: conn });
  }

  function resetFilters() {
    searchQuery = '';
    selectedProjectFilter = 'all';
    selectedConnectionFilter = 'all';
    selectedStatusFilter = 'all';
  }
</script>

<div class="view-full-height">
  <!-- Page Header -->
  <section class="page-heading history-page-heading">
    <div>
      <div class="heading-breadcrumbs">Deployments</div>
      <h1>Deployment History</h1>
      <p class="page-subtitle">
        Independent execution history, verification records, and verified rollback recovery.
      </p>
    </div>

    <div class="heading-actions">
      <!-- Restrained Summary Strip -->
      <div class="metrics-strip">
        <div class="metric-pill" title="Total recorded deployment executions">
          <span class="metric-num">{totalCount}</span>
          <span class="metric-lbl">Total</span>
        </div>
        <div class="metric-pill text-success" title="Successfully verified deployments">
          <span class="metric-num">{successCount}</span>
          <span class="metric-lbl">Success</span>
        </div>
        {#if failedCount > 0}
          <div class="metric-pill text-danger" title="Failed or interrupted deployments">
            <span class="metric-num">{failedCount}</span>
            <span class="metric-lbl">Failed</span>
          </div>
        {/if}
        {#if activeCount > 0}
          <div class="metric-pill text-info" title="Currently running deployments">
            <span class="metric-num">{activeCount}</span>
            <span class="metric-lbl">Active</span>
          </div>
        {/if}
      </div>

      <button
        class="btn btn-primary"
        on:click={() => dispatch('newDeployment')}
        disabled={$deployStatus === 'running' || !projects.length || !connections.length}
      >
        <Icon name="play" size={14} />
        <span>New Deployment</span>
      </button>
    </div>
  </section>

  <div class="view-scrollable">
    <div class="card page-card">
      <!-- Filter Bar -->
      <div class="card-header history-filter-bar">
        <div class="filter-controls-wrap">
          <div class="search-box">
            <Icon name="search" size={14} />
            <input
              class="form-input search-input"
              placeholder="Search by ID, project, target host, or error..."
              bind:value={searchQuery}
              aria-label="Search deployment history"
            />
            {#if searchQuery}
              <button
                class="search-clear-btn"
                on:click={() => (searchQuery = '')}
                title="Clear search"
              >
                <Icon name="close" size={12} />
              </button>
            {/if}
          </div>

          <!-- Project Filter -->
          <select
            class="form-select filter-select"
            bind:value={selectedProjectFilter}
            aria-label="Filter by project"
          >
            <option value="all">All Projects</option>
            {#each projects as p}
              <option value={p.name}>{p.name}</option>
            {/each}
          </select>

          <!-- Connection Filter -->
          <select
            class="form-select filter-select"
            bind:value={selectedConnectionFilter}
            aria-label="Filter by connection"
          >
            <option value="all">All Connections</option>
            {#each connections as c}
              <option value={c.name}>{c.name}</option>
            {/each}
          </select>

          <!-- Status Filter -->
          <select
            class="form-select filter-select"
            bind:value={selectedStatusFilter}
            aria-label="Filter by status"
          >
            <option value="all">All Statuses</option>
            <option value="success">Success</option>
            <option value="failed">Failed</option>
            <option value="cancelled">Cancelled</option>
            <option value="rolled_back">Rolled Back</option>
            <option value="running">In Progress</option>
            <option value="interrupted">Interrupted</option>
          </select>

          <button
            class="btn btn-secondary btn-sm"
            on:click={() => dispatch('refresh')}
            disabled={loading}
            title="Refresh history"
          >
            <Icon name="refresh" size={14} spin={loading} />
            <span>Refresh</span>
          </button>
        </div>
      </div>

      <!-- Scannable Table Body -->
      <div class="card-body" style="padding: 0;">
        {#if loading && deployments.length === 0}
          <div class="loading-state">
            <Icon name="loader" size={24} spin={true} />
            <p>Loading deployment history...</p>
          </div>
        {:else if deployments.length === 0}
          <!-- Zero deployments empty state -->
          <div class="empty-state">
            <Icon name="deploy" size={36} />
            <h3>No Deployments Yet</h3>
            <p>
              Publish your Angular or .NET build artifacts to FTP/FTPS with automated backups, SHA-256 verification, and 1-click rollback.
            </p>
            <button
              class="btn btn-primary mt-2"
              on:click={() => dispatch('newDeployment')}
              disabled={!projects.length || !connections.length}
            >
              <Icon name="play" size={14} />
              <span>Start First Deployment</span>
            </button>
          </div>
        {:else if filteredDeployments.length === 0}
          <!-- Filtered out state -->
          <div class="empty-filter-state">
            <p class="text-muted">No deployment runs match your filter criteria.</p>
            <button class="btn btn-secondary btn-sm mt-2" on:click={resetFilters}>
              Reset Filters
            </button>
          </div>
        {:else}
          <div class="table-wrap">
            <table class="history-table">
              <thead>
                <tr>
                  <th style="width: 85px;">ID</th>
                  <th style="width: 140px;">Status</th>
                  <th>Project</th>
                  <th>Target Host</th>
                  <th>Started</th>
                  <th>Duration</th>
                  <th style="text-align: right; width: 120px;">Actions</th>
                </tr>
              </thead>
              <tbody>
                {#each filteredDeployments as d (d.id)}
                  {@const status = resolveDeploymentStatus(
                    d,
                    $activeDeploymentId,
                    $deployStatus === 'running',
                    activeRollbackId,
                    failedRollbackIds
                  )}
                  {@const projName =
                    d.projectName ||
                    projects.find((p) => p.id === d.projectId)?.name ||
                    `Project #${d.projectId}`}
                  {@const projType =
                    d.projectType ||
                    projects.find((p) => p.id === d.projectId)?.projectType ||
                    'generic'}
                  {@const connName =
                    d.connectionName ||
                    connections.find((c) => c.id === d.connectionId)?.name ||
                    `Connection #${d.connectionId}`}
                  {@const connHost =
                    d.connectionHost ||
                    connections.find((c) => c.id === d.connectionId)?.host ||
                    ''}

                  <tr
                    class="history-row"
                    on:click={() => dispatch('selectRun', d.id)}
                    title="View run execution details and logs"
                  >
                    <!-- Run ID -->
                    <td class="text-mono">
                      <span class="run-id-pill">#{d.id}</span>
                    </td>

                    <!-- Status Badge -->
                    <td>
                      <span class="badge {status.badgeClass}">
                        <Icon name={status.icon} size={12} spin={status.spin} />
                        <span>{status.label}</span>
                      </span>
                    </td>

                    <!-- Project -->
                    <td>
                      <div class="project-cell">
                        <span class="project-type-icon">
                          <Icon name={getProjectIcon(projType)} size={14} />
                        </span>
                        <strong>{projName}</strong>
                      </div>
                    </td>

                    <!-- Target Connection -->
                    <td>
                      <div class="connection-cell">
                        <span>{connName}</span>
                        {#if connHost}
                          <span class="text-xs text-muted text-mono">({connHost})</span>
                        {/if}
                      </div>
                    </td>

                    <!-- Started At -->
                    <td class="text-sm">
                      {formatDateTime(d.startedAt)}
                    </td>

                    <!-- Duration -->
                    <td class="text-sm text-mono text-muted">
                      {formatDuration(d.startedAt, d.finishedAt)}
                    </td>

                    <!-- Contextual Actions -->
                    <td style="text-align: right;" on:click|stopPropagation>
                      <div class="row-actions">
                        <!-- View Details -->
                        <button
                          class="table-icon-action action-secondary"
                          on:click={() => dispatch('selectRun', d.id)}
                          title="Inspect run details, logs, and files"
                          aria-label={`Inspect deployment #${d.id}`}
                        >
                          <Icon name="search" size={14} />
                        </button>

                        <!-- Deploy with same config (New Run) -->
                        <button
                          class="table-icon-action action-primary"
                          on:click={(e) => handleDeployAgain(d, e)}
                          disabled={$deployStatus === 'running'}
                          title="Start new deployment using this configuration"
                          aria-label={`Deploy again using configuration from #${d.id}`}
                        >
                          <Icon name="play" size={14} />
                        </button>

                        <!-- Rollback button only shown if this is the most recent failed deployment -->
                        {#if isRollbackPossible(d, deployments)}
                          <button
                            class="table-icon-action action-danger"
                            on:click={(e) => handleRowRollback(d, e)}
                            disabled={activeRollbackId === d.id}
                            title="Restore remote files from backup snapshot"
                            aria-label={`Roll back deployment #${d.id}`}
                          >
                            <Icon
                              name="rollback"
                              size={15}
                              spin={activeRollbackId === d.id}
                            />
                          </button>
                        {/if}
                      </div>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    </div>
  </div>
</div>

<style>
  .history-page-heading {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    flex-wrap: wrap;
    gap: 16px;
    margin-bottom: 16px;
  }

  .heading-breadcrumbs {
    font-size: 11px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--color-primary);
    margin-bottom: 2px;
  }

  .heading-actions {
    display: flex;
    align-items: center;
    gap: 14px;
    flex-wrap: wrap;
  }

  /* Restrained Summary Strip */
  .metrics-strip {
    display: inline-flex;
    align-items: center;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    padding: 3px 6px;
    gap: 12px;
  }

  .metric-pill {
    display: flex;
    align-items: baseline;
    gap: 4px;
    padding: 2px 6px;
  }

  .metric-num {
    font-size: 13px;
    font-weight: 700;
  }

  .metric-lbl {
    font-size: 10px;
    font-weight: 600;
    color: var(--color-text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .history-filter-bar {
    background: #f8fafc;
    border-bottom: 1px solid var(--color-border);
    padding: 10px 16px;
  }

  .filter-controls-wrap {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    flex-wrap: wrap;
  }

  .search-box {
    position: relative;
    display: flex;
    align-items: center;
    flex: 1;
    min-width: 220px;
  }

  .search-box :global(.lucide-icon-wrap) {
    position: absolute;
    left: 10px;
    color: var(--color-text-light);
    pointer-events: none;
  }

  .search-input {
    padding-left: 32px;
    padding-right: 28px;
    font-size: 12.5px;
    background: #ffffff;
  }

  .search-clear-btn {
    position: absolute;
    right: 8px;
    background: transparent;
    border: none;
    cursor: pointer;
    color: var(--color-text-light);
    display: flex;
    align-items: center;
    padding: 2px;
  }

  .filter-select {
    max-width: 170px;
    font-size: 12.5px;
    background-color: #ffffff;
  }

  .history-table {
    margin: 0;
  }

  .history-row {
    cursor: pointer;
    transition: background-color 0.12s ease;
  }

  .history-row:hover td {
    background-color: #f1f5f9;
  }

  .run-id-pill {
    font-weight: 600;
    color: var(--color-primary);
  }

  .project-cell {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .project-type-icon {
    color: var(--color-text-secondary);
    display: inline-flex;
    align-items: center;
  }

  .connection-cell {
    display: flex;
    align-items: baseline;
    gap: 6px;
  }

  .row-actions {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  .empty-state {
    padding: 48px 24px;
    text-align: center;
    color: var(--color-text-secondary);
  }

  .empty-state h3 {
    margin: 12px 0 6px;
    font-size: 16px;
    color: var(--color-text);
  }

  .empty-state p {
    max-width: 460px;
    margin: 0 auto 16px;
    font-size: 13px;
    line-height: 1.5;
  }

  .empty-filter-state {
    padding: 36px 16px;
    text-align: center;
  }

  .loading-state {
    padding: 48px 16px;
    text-align: center;
    color: var(--color-text-secondary);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
  }
</style>

