<script lang="ts">
  import { onMount } from 'svelte';
  import type { ProjectDTO } from '../types';
  import { projects, deploymentHistory, currentPage, selectedProjectName, newProjectRequested, addToast } from '../stores';
  import { getProjects, getDeploymentHistory, deleteProject } from '../wailsjs';
  import ProjectForm from './ProjectForm.svelte';

  let showForm = false;
  let editingProject: ProjectDTO | null = null;
  let loading = true;

  onMount(async () => {
    await loadProjects();
    if ($newProjectRequested) { $newProjectRequested = false; handleAdd(); }
  });

  async function loadProjects() {
    loading = true;
    try {
      $projects = await getProjects();
      $deploymentHistory = await getDeploymentHistory(100);
    } catch (err: any) {
      addToast('error', `Failed to load projects: ${err.message}`);
    } finally {
      loading = false;
    }
  }

  function handleAdd() {
    editingProject = null;
    showForm = true;
  }

  function handleEdit(p: ProjectDTO) {
    editingProject = { ...p };
    showForm = true;
  }

  async function handleDelete(p: ProjectDTO) {
    if (!confirm(`Are you sure you want to delete project "${p.name}"?`)) {
      return;
    }
    try {
      await deleteProject(p.id);
      addToast('success', `Project "${p.name}" deleted`);
      await loadProjects();
    } catch (err: any) {
      addToast('error', `Failed to delete project: ${err.message}`);
    }
  }

  function handleDeploy(p: ProjectDTO) {
    $selectedProjectName = p.name;
    $currentPage = 'deploy';
  }

  function handleSaved() {
    showForm = false;
    editingProject = null;
    loadProjects();
  }

  function handleCancel() {
    showForm = false;
    editingProject = null;
  }
</script>

<div class="card">
  <div class="card-header">
    <div>
      <h2>Projects</h2>
      <p class="text-sm text-muted">Configure Angular and .NET application build & artifact paths</p>
    </div>
    <button class="btn btn-primary" on:click={handleAdd}>
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <line x1="12" y1="5" x2="12" y2="19" />
        <line x1="5" y1="12" x2="19" y2="12" />
      </svg>
      <span>Add Project</span>
    </button>
  </div>

  <div class="card-body" style="padding: 0;">
    {#if loading}
      <div style="padding: 32px; text-align: center;" class="text-muted">Loading projects...</div>
    {:else if $projects.length === 0}
      <div class="empty-state">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 8 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z" />
        </svg>
        <h3>No projects yet</h3>
        <p>Define an Angular or .NET project with its build command and output directory.</p>
        <button class="btn btn-primary" on:click={handleAdd}>Add First Project</button>
      </div>
    {:else}
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Type</th>
              <th>Local Path</th>
              <th>Build Command</th>
              <th>Output Rel Path</th>
              <th>Last deployment</th>
              <th style="text-align: right;">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each $projects as p (p.id)}
              <tr>
                <td>
                  <strong>{p.name}</strong>
                </td>
                <td>
                  {#if p.projectType === 'angular'}
                    <span class="badge badge-info">Angular</span>
                  {:else}
                    <span class="badge badge-success">.NET</span>
                  {/if}
                </td>
                <td class="text-mono text-muted truncate" style="max-width: 200px;" title={p.localPath}>
                  {p.localPath}
                </td>
                <td class="text-mono text-muted truncate" style="max-width: 160px;" title={p.buildCommand}>
                  {p.buildCommand || '(none)'}
                </td>
                <td class="text-mono text-muted">{p.outputRelPath}</td>
                <td>{#if $deploymentHistory.find(d => d.projectId === p.id)}{@const last = $deploymentHistory.find(d => d.projectId === p.id)}<span class="activity-status" class:success={last?.status === 'completed'} class:failed={last?.status === 'failed' && !last?.cancelled}><i></i>{last?.cancelled ? 'Cancelled' : last?.status === 'completed' ? 'Success' : last?.status === 'failed' ? 'Failed' : last?.status}</span>{:else}<span class="text-muted">No runs</span>{/if}</td>
                <td style="text-align: right; white-space: nowrap;">
                  <button
                    class="btn btn-primary btn-sm"
                    on:click={() => handleDeploy(p)}
                    title="Deploy this project"
                  >
                    Deploy
                  </button>
                  <button
                    class="btn btn-ghost btn-sm"
                    on:click={() => handleEdit(p)}
                    title="Edit project"
                  >
                    Edit
                  </button>
                  <button
                    class="btn btn-ghost btn-sm"
                    style="color: var(--color-danger);"
                    on:click={() => handleDelete(p)}
                    title="Delete project"
                  >
                    Delete
                  </button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</div>

{#if showForm}
  <ProjectForm
    project={editingProject}
    on:saved={handleSaved}
    on:cancel={handleCancel}
  />
{/if}

