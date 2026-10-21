<script lang="ts">
  import Icon from './Icon.svelte';
  import { getStatusIcon, getFileIcon } from '../icons';
  import { onMount } from 'svelte';
  import type { DeploymentDTO, DeploymentFileDTO } from '../types';
  import {
    deploymentHistory,
    connections,
    projects,
    addToast,
  } from '../stores';
  import {
    getDeploymentHistory,
    getDeploymentFiles,
    rollbackDeployment,
    getConnections,
    getProjects,
  } from '../wailsjs';

  let loading = true;
  let expandedDeploymentId: number | null = null;
  let deploymentFiles: DeploymentFileDTO[] = [];
  let loadingFiles = false;
  let rollingBackId: number | null = null;
  let searchText = '';
  let statusFilter = 'all';
  $: filteredHistory = $deploymentHistory.filter((d) => {
    const query = searchText.trim().toLowerCase();
    const projectName =
      $projects.find((p) => p.id === d.projectId)?.name ??
      `Project #${d.projectId}`;
    const connectionName =
      $connections.find((c) => c.id === d.connectionId)?.name ??
      `Connection #${d.connectionId}`;
    const matchesText =
      !query ||
      `${projectName} ${connectionName} ${d.status} ${d.currentPhase} ${d.errorMessage}`
        .toLowerCase()
        .includes(query);
    const matchesStatus =
      statusFilter === 'all' ||
      (statusFilter === 'cancelled'
        ? d.cancelled === true || d.status === 'cancelled'
        : d.status === statusFilter);
    return matchesText && matchesStatus;
  });

  onMount(async () => {
    try {
      [$projects, $connections] = await Promise.all([
        getProjects(),
        getConnections(),
      ]);
    } catch {
      /* history remains available if profile lookup fails */
    }
    await loadHistory();
  });

  async function loadHistory() {
    loading = true;
    try {
      $deploymentHistory = await getDeploymentHistory(50);
    } catch (err: any) {
      addToast('error', `Failed to load history: ${err.message}`);
    } finally {
      loading = false;
    }
  }

  async function toggleExpand(d: DeploymentDTO) {
    if (expandedDeploymentId === d.id) {
      expandedDeploymentId = null;
      deploymentFiles = [];
      return;
    }

    expandedDeploymentId = d.id;
    loadingFiles = true;
    try {
      // Load both backup and upload files
      const uploads = await getDeploymentFiles(d.id, 'upload');
      const backups = await getDeploymentFiles(d.id, 'backup');
      deploymentFiles = [...uploads, ...backups];
    } catch (err: any) {
      addToast('error', `Failed to load journal files: ${err.message}`);
    } finally {
      loadingFiles = false;
    }
  }

  async function handleRollback(d: DeploymentDTO) {
    const connName =
      $connections.find((c) => c.id === d.connectionId)?.name ??
      `Connection #${d.connectionId}`;
    if (
      !confirm(
        `Are you sure you want to rollback deployment #${d.id} on "${connName}"? Remote files will be restored from backup directory: ${d.backupDir}`
      )
    ) {
      return;
    }

    rollingBackId = d.id;
    try {
      await rollbackDeployment(d.id, connName);
      addToast('success', `Deployment #${d.id} rolled back successfully`);
      await loadHistory();
    } catch (err: any) {
      addToast('error', `Rollback failed: ${err.message}`);
    } finally {
      rollingBackId = null;
    }
  }

  function formatTime(iso: string): string {
    if (!iso) return '-';
    try {
      const date = new Date(iso);
      return date.toLocaleString();
    } catch {
      return iso;
    }
  }

  function getDuration(start: string, finish: string): string {
    if (!start || !finish) return '-';
    try {
      const s = new Date(start).getTime();
      const f = new Date(finish).getTime();
      const sec = Math.round((f - s) / 1000);
      if (sec < 60) return `${sec}s`;
      const min = Math.floor(sec / 60);
      const rem = sec % 60;
      return `${min}m ${rem}s`;
    } catch {
      return '-';
    }
  }
</script>

<div class="card page-card">
  <div class="card-header">
    <div>
      <h2>Deployment History</h2>
      <p class="text-sm text-muted">
        Audit trail of all deployments, status journals, and explicit rollbacks
      </p>
    </div>
    <div class="history-controls">
      <input
        class="form-input history-search"
        aria-label="Search history"
        placeholder="Search project, status, or phase"
        bind:value={searchText}
      /><select
        class="form-select history-filter"
        aria-label="Filter by status"
        bind:value={statusFilter}
        ><option value="all">All statuses</option><option value="completed"
          >Success</option
        ><option value="failed">Failed</option><option value="cancelled"
          >Cancelled</option
        ><option value="rolled_back">Rolled back</option><option value="running"
          >Running</option
        ></select
      ><button
        class="btn btn-secondary btn-sm"
        on:click={loadHistory}
        disabled={loading}
      >
        <Icon name="refresh" size={16} />
        <span>Refresh</span>
      </button>
    </div>
  </div>

  <div class="card-body" style="padding: 0;">
    {#if loading}
      <div style="padding: 32px; text-align: center;" class="text-muted">
        Loading history...
      </div>
    {:else if $deploymentHistory.length === 0}
      <div class="empty-state">
        <Icon name="history" size={24} />
        <h3>No deployment records</h3>
        <p>Your deployment history and file journals will appear here.</p>
      </div>
    {:else}
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>Status</th>
              <th>Project</th>
              <th>Connection</th>
              <th>Phase</th>
              <th>Started At</th>
              <th>Duration</th>
              <th style="text-align: right;">Action</th>
            </tr>
          </thead>
          <tbody>
            {#each filteredHistory as d (d.id)}
              <tr style="cursor: pointer;" on:click={() => toggleExpand(d)}>
                <td class="text-mono">#{d.id}</td>
                <td>
                  {#if d.cancelled}
                    <span class="badge badge-warning"><Icon name="warning" size={12} /> Cancelled</span>
                  {:else if d.status === 'completed'}
                    <span class="badge badge-success"><Icon name="success" size={12} /> Completed</span>
                  {:else if d.status === 'failed'}
                    <span class="badge badge-danger"><Icon name="error" size={12} /> Failed</span>
                  {:else if d.status === 'rolled_back'}
                    <span class="badge badge-gray"><Icon name="rollback" size={12} /> Rolled Back</span>
                  {:else if d.status === 'cancelled'}
                    <span class="badge badge-warning"><Icon name="warning" size={12} /> Cancelled</span>
                  {:else}
                    <span class="badge badge-warning"><Icon name="loader" size={12} /> {d.status}</span>
                  {/if}
                </td>
                <td
                  ><strong
                    >{$projects.find((p) => p.id === d.projectId)?.name ??
                      `Project #${d.projectId}`}</strong
                  ></td
                >
                <td
                  >{$connections.find((c) => c.id === d.connectionId)?.name ??
                    `Connection #${d.connectionId}`}</td
                >
                <td class="text-mono text-muted">{d.currentPhase}</td>
                <td class="text-sm">{formatTime(d.startedAt)}</td>
                <td class="text-sm text-muted"
                  >{getDuration(d.startedAt, d.finishedAt)}</td
                >
                <td style="text-align: right;" on:click|stopPropagation>
                  {#if d.status === 'failed' || d.status === 'running' || d.cancelled || d.status === 'cancelled'}
                    <button
                      class="table-icon-action action-danger"
                      on:click={() => handleRollback(d)}
                      disabled={rollingBackId === d.id}
                      title={rollingBackId === d.id ? 'Rolling back...' : 'Restore remote files from backup (Rollback)'}
                      aria-label={`Rollback deployment #${d.id}`}
                    >
                      <Icon name="rollback" size={17} spin={rollingBackId === d.id} />
                    </button>
                  {/if}
                </td>
              </tr>

              <!-- Expanded file details accordion -->
              {#if expandedDeploymentId === d.id}
                <tr>
                  <td
                    colspan="8"
                    style="background: var(--color-input-bg); padding: 16px 24px;"
                  >
                    <div style="margin-bottom: 8px;">
                      <strong>Deployment Details</strong>
                      {#if d.errorMessage}
                        <div
                          style="margin-top: 4px; color: var(--color-danger); font-size: 13px;"
                        >
                          <strong>Error:</strong>
                          {d.errorMessage}
                        </div>
                      {/if}
                      <div
                        class="text-sm text-muted text-mono"
                        style="margin-top: 4px;"
                      >
                        Backup Directory: {d.backupDir}
                      </div>
                    </div>

                    <div style="margin-top: 12px;">
                      <div
                        style="font-weight: 600; font-size: 12px; margin-bottom: 6px; text-transform: uppercase; color: var(--color-text-secondary);"
                      >
                        Journaled Operations ({deploymentFiles.length} files)
                      </div>
                      {#if loadingFiles}
                        <div class="text-sm text-muted">
                          Loading file journal...
                        </div>
                      {:else if deploymentFiles.length === 0}
                        <div class="text-sm text-muted">
                          No journaled file entries recorded for this
                          deployment.
                        </div>
                      {:else}
                        <div
                          style="max-height: 200px; overflow-y: auto; background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-sm);"
                        >
                          <table>
                            <thead>
                              <tr>
                                <th>Stage</th>
                                <th>Remote Path</th>
                                <th>Size</th>
                              </tr>
                            </thead>
                            <tbody>
                              {#each deploymentFiles as f}
                                <tr>
                                  <td>
                                    <span
                                      class="badge {f.stage === 'upload'
                                        ? 'badge-info'
                                        : 'badge-gray'}"
                                    >
                                      <Icon name={f.stage === 'upload' ? 'upload' : 'archive'} size={12} />
                                      {f.stage} · {f.status}
                                    </span>
                                  </td>
                                  <td class="text-mono text-sm"
                                    ><span class="file-icon" style="margin-right: 6px;"><Icon name={getFileIcon(f.relativePath, false)} size={14} /></span>{f.relativePath}</td
                                  >
                                  <td class="text-mono text-sm text-muted"
                                    >{f.sizeBytes} B</td
                                  >
                                </tr>
                              {/each}
                            </tbody>
                          </table>
                        </div>
                      {/if}
                    </div>
                  </td>
                </tr>
              {/if}
            {/each}
            {#if filteredHistory.length === 0}<tr
                ><td colspan="8" class="text-muted"
                  >No history entries match these filters.</td
                ></tr
              >{/if}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</div>
