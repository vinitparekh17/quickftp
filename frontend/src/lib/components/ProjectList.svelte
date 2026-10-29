<script lang="ts">
  import Icon from './Icon.svelte';
  import { getProjectIcon } from '../icons';
  import { onMount, tick } from 'svelte';
  import type { ProjectDTO } from '../types';
  import {
    projects,
    deploymentHistory,
    currentPage,
    requestedDeploymentId,
    deployStatus,
    deploymentProjectId,
    selectedProjectName,
    newProjectRequested,
    addToast,
  } from '../stores';
  import { getProjects, getDeploymentHistory, deleteProject } from '../wailsjs';
  import ProjectForm from './ProjectForm.svelte';

  let showForm = false;
  let editingProject: ProjectDTO | null = null;
  let loading = true;

  let menuProject: ProjectDTO | null = null;
  let menuElement: HTMLDivElement;
  let menuPosition = '';
  let actionTrigger: HTMLButtonElement | null = null;

  function closeMenu() {
    menuProject = null;
  }
  async function toggleActions(project: ProjectDTO, event: MouseEvent) {
    event.stopPropagation();
    if (menuProject?.id === project.id) {
      closeMenu();
      return;
    }
    actionTrigger = event.currentTarget as HTMLButtonElement;
    const rect = actionTrigger.getBoundingClientRect();
    const width = 245,
      height = 186,
      gap = 6;
    const left = Math.max(
      12,
      Math.min(rect.right - width, window.innerWidth - width - 12)
    );
    const top =
      rect.bottom + height + gap <= window.innerHeight
        ? rect.bottom + gap
        : Math.max(12, rect.top - height - gap);
    menuPosition = `left:${left}px;top:${top}px;width:${width}px`;
    menuProject = project;
    await tick();
    menuElement
      ?.querySelector<HTMLButtonElement>('button:not(:disabled)')
      ?.focus();
  }
  function outsideClick(event: MouseEvent) {
    if (menuProject && !menuElement?.contains(event.target as Node))
      closeMenu();
  }
  function menuKeydown(event: KeyboardEvent) {
    if (!menuProject) return;
    if (event.key === 'Escape' || event.key === 'Tab') {
      if (event.key === 'Escape') event.preventDefault();
      closeMenu();
      actionTrigger?.focus();
      return;
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault();
    const buttons = Array.from(
      menuElement.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')
    );
    const index = buttons.indexOf(document.activeElement as HTMLButtonElement);
    const next =
      event.key === 'Home'
        ? 0
        : event.key === 'End'
          ? buttons.length - 1
          : (index + (event.key === 'ArrowUp' ? -1 : 1) + buttons.length) %
            buttons.length;
    buttons[next]?.focus();
  }
  function projectAction(action: 'deploy' | 'history' | 'edit' | 'delete') {
    const project = menuProject;
    closeMenu();
    if (!project) return;
    if (action === 'deploy') handleDeploy(project);
    else if (action === 'history') {
      $deploymentProjectId = project.id;
      $selectedProjectName = project.name;
      $requestedDeploymentId = null;
      $currentPage = 'history';
    } else if (action === 'edit') handleEdit(project);
    else void handleDelete(project);
  }

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
    $requestedDeploymentId = null;
    $deploymentProjectId = p.id;
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

<svelte:window
  on:click={outsideClick}
  on:keydown={menuKeydown}
  on:resize={closeMenu}
  on:scroll|capture={closeMenu}
/>

<div class="view-full-height">
  <section class="page-heading">
    <div>
      <h1>Projects</h1>
      <p class="page-subtitle">
        Build commands, output paths, and recent deployment results.
      </p>
    </div>
    <button class="btn btn-primary" on:click={handleAdd}
      ><Icon name="plus" size={15} /> Add project</button
    >
  </section>
  <div class="card page-card resource-list-card">
    {#if loading}<div class="files-state">
        <Icon name="loader" size={20} spin /> Loading projects…
      </div>
    {:else if !$projects.length}<div class="empty-state">
        <Icon name="projects" size={28} />
        <h3>No projects yet</h3>
        <p>
          Add an Angular or .NET project to define what QuickFTP should build
          and publish.
        </p>
        <button class="btn btn-primary" on:click={handleAdd}>Add project</button
        >
      </div>
    {:else}<div class="table-wrap">
        <table class="resource-table">
          <thead
            ><tr
              ><th>Project</th><th>Build &amp; output</th><th>Latest run</th><th
                class="row-actions-heading">Actions</th
              ></tr
            ></thead
          ><tbody>
            {#each $projects as p (p.id)}
              {@const last = $deploymentHistory.find(
                (run) => run.projectId === p.id
              )}
              <tr
                ><td
                  ><div class="resource-name">
                    <Icon
                      name={getProjectIcon(p.projectType)}
                      size={16}
                    /><button
                      class="run-link"
                      on:click={() => {
                        $deploymentProjectId = p.id;
                        $selectedProjectName = p.name;
                        $requestedDeploymentId = null;
                        $currentPage = 'history';
                      }}>{p.name}</button
                    ><span class="resource-kind"
                      >{p.projectType === 'angular' ? 'Angular' : '.NET'}</span
                    >
                  </div>
                  <span class="cell-secondary text-mono resource-path"
                    >{p.localPath}</span
                  ></td
                >
                <td
                  ><span class="text-mono resource-path"
                    >{p.buildCommand || 'Skip build'}</span
                  ><span class="cell-secondary"
                    >Output: <span class="text-mono">{p.outputRelPath}</span
                    ></span
                  ></td
                >
                <td
                  >{#if last}<button
                      class="run-link"
                      on:click={() => {
                        $deploymentProjectId = p.id;
                        $requestedDeploymentId = last.id;
                        $currentPage = 'history';
                      }}
                      ><span
                        class="activity-status"
                        class:success={last.status === 'completed'}
                        class:failed={last.status === 'failed' &&
                          !last.cancelled}
                        >{last.cancelled
                          ? 'Cancelled'
                          : last.status === 'completed'
                            ? 'Success'
                            : last.status === 'failed'
                              ? 'Failed'
                              : last.status}</span
                      ><span>#{last.id}</span></button
                    >{:else}<span class="text-muted">No runs</span>{/if}</td
                >
                <td
                  ><div class="table-actions">
                    <button
                      class="btn btn-secondary btn-sm"
                      aria-label={`Actions for ${p.name}`}
                      aria-haspopup="menu"
                      aria-controls="project-actions-menu"
                      aria-expanded={menuProject?.id === p.id}
                      on:click={(event) => toggleActions(p, event)}
                    >
                      Actions <Icon name="chevronDown" size={13} />
                    </button>
                  </div></td
                ></tr
              >
            {/each}
          </tbody>
        </table>
      </div>{/if}
  </div>
</div>
{#if showForm}<ProjectForm
    project={editingProject}
    on:saved={handleSaved}
    on:cancel={handleCancel}
  />{/if}

{#if menuProject}
  <div
    id="project-actions-menu"
    class="project-actions-menu"
    role="menu"
    aria-label={`Actions for ${menuProject.name}`}
    style={menuPosition}
    bind:this={menuElement}
  >
    <button
      role="menuitem"
      on:click={() => projectAction('deploy')}
      disabled={$deployStatus === 'running'}
      ><Icon name="play" size={15} /><span>Deploy</span></button
    >
    <button role="menuitem" on:click={() => projectAction('history')}
      ><Icon name="history" size={15} /><span>View deployments &amp; logs</span
      ></button
    >
    <button role="menuitem" on:click={() => projectAction('edit')}
      ><Icon name="edit" size={15} /><span>Edit project</span></button
    >
    <button
      role="menuitem"
      class="destructive-action"
      on:click={() => projectAction('delete')}
      ><Icon name="trash" size={15} /><span>Delete project</span></button
    >
  </div>
{/if}

<style>
  .project-actions-menu {
    position: fixed;
    z-index: 1100;
    padding: 6px;
    border: 1px solid var(--color-border);
    border-radius: 8px;
    background: var(--color-surface);
    box-shadow: 0 8px 24px #15284024;
  }
  .project-actions-menu button {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 10px 12px;
    border: 0;
    border-radius: 4px;
    background: transparent;
    color: var(--color-text);
    font: inherit;
    font-size: 12px;
    text-align: left;
    cursor: pointer;
  }
  .project-actions-menu button:hover:not(:disabled),
  .project-actions-menu button:focus-visible {
    background: #edf3fb;
    outline: none;
  }
  .project-actions-menu .destructive-action {
    color: var(--color-danger);
    border-top: 1px solid var(--color-border);
    border-radius: 0 0 4px 4px;
    margin-top: 4px;
  }
  .project-actions-menu button:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }
</style>
