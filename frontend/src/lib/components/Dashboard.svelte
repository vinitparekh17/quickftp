<script lang="ts">
  import Icon from './Icon.svelte';
  import { getProjectIcon, getStatusIcon } from '../icons';
  import { formatDuration, formatDateTime } from '../deploymentUtils';
  import { onMount } from 'svelte';
  import {
    currentPage,
    connections,
    projects,
    deploymentHistory,
    selectedProjectName,
    selectedConnectionName,
    newProjectRequested,
    requestedDeploymentId,
    deployStatus,
    activeDeployment,
    addToast,
  } from '../stores';
  import {
    getConnections,
    getProjects,
    getDeploymentHistory,
  } from '../wailsjs';

  let projectName = $selectedProjectName;
  let connectionName = $selectedConnectionName;
  let loading = true;
  $: project = $projects.find((item) => item.name === projectName);
  $: connection = $connections.find((item) => item.name === connectionName);
  onMount(async () => {
    try {
      const [p, c, history] = await Promise.all([
        getProjects(),
        getConnections(),
        getDeploymentHistory(6),
      ]);
      $projects = p;
      $connections = c;
      $deploymentHistory = history.slice().sort((a, b) => b.id - a.id);
      if (!p.some((item) => item.name === projectName))
        projectName = p[0]?.name || '';
      if (!c.some((item) => item.name === connectionName))
        connectionName = c[0]?.name || '';
    } catch (error: any) {
      addToast('error', `Could not load dashboard: ${error.message}`);
    } finally {
      loading = false;
    }
  });
  function startDeploy() {
    $selectedProjectName = projectName;
    $selectedConnectionName = connectionName;
    $currentPage = 'deploy';
  }
  function openRun(id: number) {
    $requestedDeploymentId = id;
    $currentPage = 'history';
  }
  function createProject() {
    $newProjectRequested = true;
    $currentPage = 'projects';
  }
  function statusLabel(status: string, cancelled = false) {
    if (cancelled) return 'Cancelled';
    return (
      (
        {
          completed: 'Success',
          failed: 'Failed',
          running: 'In progress',
          rolled_back: 'Rolled back',
        } as Record<string, string>
      )[status] || status
    );
  }
</script>

<div class="view-full-height dashboard-page">
  <section class="page-heading">
    <div>
      <h1>Dashboard</h1>
      <p class="page-subtitle">
        Choose what to deploy and where. Review recent runs below.
      </p>
    </div>
  </section>
  <div class="view-scrollable dashboard-content">
    {#if loading}
      <div class="files-state">
        <Icon name="loader" size={20} spin /> Loading workspace…
      </div>
    {:else}
      {#if $deployStatus === 'running'}
        <div class="context-notice">
          <span
            ><Icon name="loader" size={16} spin />
            <strong>{$activeDeployment?.projectName || 'Deployment'}</strong> is running.</span
          ><button class="text-button" on:click={() => openRun(0)}
            >View live deployment <Icon name="forward" size={13} /></button
          >
        </div>
      {/if}
      <section class="card dashboard-deploy-card">
        <div class="card-header">
          <h2>Deploy a project</h2>
          <span class="text-muted text-sm"
            >Review settings before anything is published</span
          >
        </div>
        <div class="card-body">
          {#if !$projects.length || !$connections.length}
            <div class="setup-prompt">
              <h3>Set up your workspace</h3>
              <p class="text-muted">
                Add a project and a server connection to start deploying.
              </p>
              <div class="setup-actions">
                {#if !$projects.length}<button
                    class="btn btn-primary"
                    on:click={createProject}
                    ><Icon name="plus" size={14} /> Add project</button
                  >{/if}{#if !$connections.length}<button
                    class="btn btn-secondary"
                    on:click={() => ($currentPage = 'connections')}
                    ><Icon name="server" size={14} /> Set up connection</button
                  >{/if}
              </div>
            </div>
          {:else}
            <div class="deploy-quick-form">
              <div class="form-group">
                <label class="form-label" for="dashboard-project">Project</label
                ><select
                  class="form-select"
                  id="dashboard-project"
                  bind:value={projectName}
                  >{#each $projects as item}<option value={item.name}
                      >{item.name}</option
                    >{/each}</select
                >
              </div>
              <Icon name="forward" size={18} />
              <div class="form-group">
                <label class="form-label" for="dashboard-connection"
                  >Target connection</label
                ><select
                  class="form-select"
                  id="dashboard-connection"
                  bind:value={connectionName}
                  >{#each $connections as item}<option value={item.name}
                      >{item.name}</option
                    >{/each}</select
                >
              </div>
              <button
                class="btn btn-primary"
                on:click={startDeploy}
                disabled={!project ||
                  !connection ||
                  $deployStatus === 'running'}
                ><Icon name="play" size={15} /> Review &amp; deploy</button
              >
            </div>
            <div class="deploy-quick-context">
              {#if project}<span
                  ><Icon
                    name={getProjectIcon(project.projectType)}
                    size={14}
                  /><span>{project.localPath}</span></span
                >{/if}
              {#if connection}<span
                  ><Icon name="server" size={14} /><span
                    >{connection.host}{connection.remoteRoot}</span
                  ></span
                >{/if}
            </div>
          {/if}
        </div>
      </section>

      <section class="section-block dashboard-recent">
        <div class="section-heading">
          <div><h2>Recent deployments</h2></div>
          <button
            class="text-button"
            on:click={() => ($currentPage = 'projects')}
            >Browse projects <Icon name="forward" size={13} /></button
          >
        </div>
        {#if $deploymentHistory.length}
          <div class="card table-wrap">
            <table class="activity-table">
              <thead
                ><tr
                  ><th>Project</th><th>Target</th><th>Status</th><th>Started</th
                  ><th>Duration</th></tr
                ></thead
              ><tbody>
                {#each $deploymentHistory.slice(0, 5) as run (run.id)}
                  <tr
                    ><td
                      ><button class="run-link" on:click={() => openRun(run.id)}
                        >{run.projectName ||
                          $projects.find((item) => item.id === run.projectId)
                            ?.name ||
                          `Project #${run.projectId}`}<span>#{run.id}</span
                        ></button
                      ></td
                    ><td
                      >{run.connectionName ||
                        $connections.find(
                          (item) => item.id === run.connectionId
                        )?.name ||
                        '—'}</td
                    ><td
                      ><span
                        class="activity-status"
                        class:success={run.status === 'completed'}
                        class:failed={run.status === 'failed' && !run.cancelled}
                        ><Icon
                          name={getStatusIcon(run.status, run.cancelled)}
                          size={12}
                        />{statusLabel(run.status, run.cancelled)}</span
                      ></td
                    ><td class="text-muted">{formatDateTime(run.startedAt)}</td
                    ><td class="text-muted"
                      >{formatDuration(run.startedAt, run.finishedAt)}</td
                    ></tr
                  >
                {/each}
              </tbody>
            </table>
          </div>
        {:else}<div class="empty-inline">
            No deployments yet. Your runs will appear here after the build
            succeeds.
          </div>{/if}
      </section>
    {/if}
  </div>
</div>
