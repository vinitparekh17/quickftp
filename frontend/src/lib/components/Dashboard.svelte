<script lang="ts">
  import { onMount } from 'svelte';
  import { currentPage, connections, projects, deploymentHistory, selectedProjectName, selectedConnectionName, newProjectRequested, addToast } from '../stores';
  import { getConnections, getProjects, getDeploymentHistory } from '../wailsjs';

  let environment = 'Production';
  let activeProjectId = '';
  let activeConnectionId = '';
  let loading = true;
  $: activeProject = $projects.find(p => String(p.id) === activeProjectId) ?? $projects[0];
  $: activeConnection = $connections.find(c => String(c.id) === activeConnectionId) ?? $connections[0];

  onMount(async () => {
    try {
      const [p, c, h] = await Promise.all([getProjects(), getConnections(), getDeploymentHistory(6)]);
      $projects = p; $connections = c; $deploymentHistory = h;
      activeProjectId = p[0] ? String(p[0].id) : '';
      activeConnectionId = c[0] ? String(c[0].id) : '';
    } catch (err: any) { addToast('error', `Could not load dashboard: ${err.message}`); }
    finally { loading = false; }
  });

  function go(page: 'projects' | 'connections' | 'deploy' | 'history') { $currentPage = page; }
  function createProject() { $newProjectRequested = true; $currentPage = 'projects'; }
  function startDeploy() { if (!activeProject) return go('projects'); $selectedProjectName = activeProject.name; $selectedConnectionName = activeConnection?.name ?? ''; $currentPage = 'deploy'; }
  function statusLabel(status: string, cancelled = false) { return cancelled ? 'Cancelled' : status === 'completed' ? 'Success' : status === 'running' ? 'Running' : status === 'failed' ? 'Failed' : status === 'rolled_back' ? 'Rolled back' : status; }
</script>

<section class="page-heading dashboard-heading">
  <div><p class="eyebrow">WORKSPACE</p><h1>Dashboard</h1><p class="page-subtitle">Your deployments, connections, and recent activity in one place.</p></div>
  <div class="dashboard-top-controls">
    <label class="compact-field"><span>Connection</span><select class="form-select" bind:value={activeConnectionId} disabled={!$connections.length}><option value="">No connection</option>{#each $connections as c}<option value={String(c.id)}>{c.name}</option>{/each}</select></label>
    <button class="btn btn-primary" on:click={createProject}>New project <span aria-hidden="true">+</span></button>
  </div>
</section>

{#if loading}<div class="loading-line">Loading workspace…</div>{/if}

<section class="quick-actions" aria-label="Quick actions">
  <button class="action-tile action-primary" on:click={startDeploy}><span class="action-icon">↑</span><span class="action-copy"><strong>Deploy project</strong><small>Build, back up, verify, and publish</small></span><span class="action-arrow">↗</span></button>
  <button class="action-tile" on:click={startDeploy}><span class="action-icon action-green">✓</span><span class="action-copy"><strong>Backup &amp; clean</strong><small>Back up, verify, then clean stale files</small></span><span class="action-arrow">↗</span></button>
  <button class="action-tile" on:click={() => go('projects')}><span class="action-icon action-slate">▣</span><span class="action-copy"><strong>Manage projects</strong><small>Configure build and publish settings</small></span><span class="action-arrow">↗</span></button>
  <button class="action-tile" on:click={() => go('connections')}><span class="action-icon action-blue">⌁</span><span class="action-copy"><strong>Connections</strong><small>Manage FTP and explicit FTPS hosts</small></span><span class="action-arrow">↗</span></button>
</section>

<section class="section-block">
  <div class="section-heading"><div><h2>Active project</h2><p>Choose an environment and start your next deployment.</p></div><button class="text-button" on:click={() => go('projects')}>All projects <span>→</span></button></div>
  {#if activeProject}
    <div class="active-project-row">
      <div class="project-mark">{activeProject.projectType === 'angular' ? 'A' : '.N'}</div>
      <div class="active-project-name"><label class="compact-field project-select-field"><span>Project</span><select class="form-select" bind:value={activeProjectId}>{#each $projects as p}<option value={String(p.id)}>{p.name}</option>{/each}</select></label><span><span class="badge badge-info">{activeProject.projectType === 'angular' ? 'Angular' : '.NET API'}</span><span class="path-label">{activeProject.localPath}</span></span></div>
      <label class="compact-field env-field"><span>Environment</span><select class="form-select" bind:value={environment}><option>Development</option><option>Production</option></select><small class="env-hint">Preset support is not connected yet</small></label>
      <div class="connection-chip"><span class="status-dot"></span><span><small>Target</small><strong>{activeConnection?.name ?? 'No connection selected'}</strong></span></div>
      <button class="btn btn-primary deploy-button" on:click={startDeploy} disabled={!activeConnection}>Deploy <span>→</span></button>
    </div>
  {:else}
    <div class="empty-inline"><span>No project configured yet.</span><button class="btn btn-primary btn-sm" on:click={() => go('projects')}>Create a project</button></div>
  {/if}
</section>

<section class="section-block activity-block">
  <div class="section-heading"><div><h2>Recent activity</h2><p>Latest deployment runs and their outcomes.</p></div><button class="text-button" on:click={() => go('history')}>View history <span>→</span></button></div>
  {#if $deploymentHistory.length}
    <div class="table-wrap"><table class="activity-table"><thead><tr><th>Project</th><th>Operation</th><th>Environment</th><th>Status</th><th>Time</th><th>Duration</th></tr></thead><tbody>
      {#each $deploymentHistory.slice(0, 5) as d (d.id)}
        <tr><td><strong>{$projects.find(p => p.id === d.projectId)?.name ?? `Project #${d.projectId}`}</strong></td><td>Deploy</td><td>—</td><td><span class="activity-status" class:success={d.status === 'completed'} class:failed={d.status === 'failed' && !d.cancelled} class:running={d.status === 'running'}><i></i>{statusLabel(d.status, d.cancelled)}</span></td><td>{d.startedAt ? new Date(d.startedAt).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—'}</td><td>{#if d.finishedAt && d.startedAt}{Math.max(1, Math.round((new Date(d.finishedAt).getTime() - new Date(d.startedAt).getTime()) / 1000))}s{:else}—{/if}</td></tr>
      {/each}
    </tbody></table></div>
  {:else}<div class="empty-inline">No deployment activity yet. Your completed runs will appear here.</div>{/if}
</section>
