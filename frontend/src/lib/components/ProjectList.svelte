<script lang="ts">
  import Icon from './Icon.svelte';
  import { getProjectIcon } from '../icons';
  import { onMount } from 'svelte';
  import type { ProjectDTO } from '../types';
  import {
    projects,
    deploymentHistory,
    currentPage,
    selectedProjectName,
    newProjectRequested,
    addToast,
  } from '../stores';
  import { getProjects, getDeploymentHistory, deleteProject } from '../wailsjs';
  import ProjectForm from './ProjectForm.svelte';

  let showForm = false;
  let editingProject: ProjectDTO | null = null;
  let loading = true;

  onMount(async () => {
    await loadProjects();
    if ($newProjectRequested) {
      $newProjectRequested = false;
      handleAdd();
    }
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

<div class="card page-card">
  <div class="card-header">
    <div>
      <h2>Projects</h2>
      <p class="text-sm text-muted">
        Configure Angular and .NET application build & artifact paths
      </p>
    </div>
    <button class="btn btn-primary" on:click={handleAdd}>
      <Icon name="plus" size={16} />
      <span>Add Project</span>
    </button>
  </div>

  <div class="card-body" style="padding: 0;">
    {#if loading}
      <div style="padding: 32px; text-align: center;" class="text-muted">
        Loading projects...
      </div>
    {:else if $projects.length === 0}
      <div class="empty-state">
        <Icon name="projects" size={24} />
        <h3>No projects yet</h3>
        <p>
          Define an Angular or .NET project with its build command and output
          directory.
        </p>
        <button class="btn btn-primary" on:click={handleAdd}
          >Add First Project</button
        >
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
                    <span class="badge badge-info"><Icon name={getProjectIcon('angular')} size={12} /> Angular</span>
                  {:else}
                    <span class="badge badge-success"><Icon name={getProjectIcon('dotnet')} size={12} /> .NET</span>
                  {/if}
                </td>
                <td
                  class="text-mono text-muted truncate"
                  style="max-width: 200px;"
                  title={p.localPath}
                >
                  {p.localPath}
                </td>
                <td
                  class="text-mono text-muted truncate"
                  style="max-width: 160px;"
                  title={p.buildCommand}
                >
                  {p.buildCommand || '(none)'}
                </td>
                <td class="text-mono text-muted">{p.outputRelPath}</td>
                <td
                  >{#if $deploymentHistory.find((d) => d.projectId === p.id)}{@const last =
                      $deploymentHistory.find((d) => d.projectId === p.id)}<span
                      class="activity-status"
                      class:success={last?.status === 'completed'}
                      class:failed={last?.status === 'failed' &&
                        !last?.cancelled}
                      ><i></i>{last?.cancelled
                        ? 'Cancelled'
                        : last?.status === 'completed'
                          ? 'Success'
                          : last?.status === 'failed'
                            ? 'Failed'
                            : last?.status}</span
                    >{:else}<span class="text-muted">No runs</span>{/if}</td
                >
                <td style="text-align: right; white-space: nowrap;">
                  <div class="table-actions">
                    <button
                      class="table-icon-action action-primary"
                      on:click={() => handleDeploy(p)}
                      title={`Deploy ${p.name}`}
                      aria-label={`Deploy ${p.name}`}
                    >
                      <Icon name="deploy" size={17} />
                    </button>
                    <button
                      class="table-icon-action action-secondary"
                      on:click={() => handleEdit(p)}
                      title={`Edit ${p.name}`}
                      aria-label={`Edit ${p.name}`}
                    >
                      <Icon name="edit" size={16} />
                    </button>
                    <button
                      class="table-icon-action action-danger"
                      on:click={() => handleDelete(p)}
                      title={`Delete ${p.name}`}
                      aria-label={`Delete ${p.name}`}
                    >
                      <Icon name="trash" size={16} />
                    </button>
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

{#if showForm}
  <ProjectForm
    project={editingProject}
    on:saved={handleSaved}
    on:cancel={handleCancel}
  />
{/if}
