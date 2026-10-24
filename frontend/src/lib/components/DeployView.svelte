<script lang="ts">
  import { onMount } from 'svelte';
  import type { DeploymentDTO } from '../types';
  import {
    deploymentHistory,
    connections,
    projects,
    deployStatus,
    activeDeploymentId,
    selectedProjectName,
    selectedConnectionName,
    currentPage,
    addToast,
  } from '../stores';
  import {
    getDeploymentHistory,
    getConnections,
    getProjects,
  } from '../wailsjs';
  import DeploymentHistory from './deploy/DeploymentHistory.svelte';
  import DeploymentDetail from './deploy/DeploymentDetail.svelte';
  import NewDeploymentModal from './deploy/NewDeploymentModal.svelte';

  // Navigation state within Deployments:
  // selectedRunId === null -> History View (default scannable table)
  // selectedRunId !== null -> Detail View for that specific execution run
  let selectedRunId: number | null = null;

  // New Deployment Modal state
  let showNewModal = false;
  let modalProject = '';
  let modalConnection = '';

  let loading = true;
  let activeRollbackId: number | null = null;
  let failedRollbackIds: Set<number> = new Set();

  $: selectedDeployment =
    $deploymentHistory.find((d) => d.id === selectedRunId) ||
    ($activeDeploymentId && selectedRunId === $activeDeploymentId
      ? {
          id: $activeDeploymentId,
          projectId:
            $projects.find((p) => p.name === $selectedProjectName)?.id ?? 0,
          projectName: $selectedProjectName,
          connectionId:
            $connections.find((c) => c.name === $selectedConnectionName)?.id ?? 0,
          connectionName: $selectedConnectionName,
          status: 'running',
          currentPhase: 'build',
          backupDir: '',
          errorMessage: '',
          startedAt: new Date().toISOString(),
          finishedAt: '',
        }
      : null);

  // If a deployment is currently running and user hasn't chosen a historical run,
  // show the active deployment details automatically.
  $: if (
    $deployStatus === 'running' &&
    $activeDeploymentId &&
    selectedRunId === null
  ) {
    selectedRunId = $activeDeploymentId;
  }

  // Pre-fill modal if requested from Dashboard / ProjectList
  $: if ($selectedProjectName && !modalProject) {
    modalProject = $selectedProjectName;
  }
  $: if ($selectedConnectionName && !modalConnection) {
    modalConnection = $selectedConnectionName;
  }

  // If user navigated directly via "Deploy" button elsewhere ($currentPage === 'deploy')
  $: if ($currentPage === 'deploy' && !showNewModal && selectedRunId === null) {
    showNewModal = true;
  }

  onMount(async () => {
    try {
      [$projects, $connections] = await Promise.all([
        getProjects(),
        getConnections(),
      ]);

      if ($projects.length > 0 && !modalProject) {
        modalProject = $projects[0].name;
      }
      if ($connections.length > 0 && !modalConnection) {
        modalConnection = $connections[0].name;
      }
    } catch (err: any) {
      console.warn('Could not load projects/connections:', err);
    }

    await loadHistory();
  });

  async function loadHistory() {
    loading = true;
    try {
      $deploymentHistory = await getDeploymentHistory(50);
    } catch (err: any) {
      addToast('error', `Could not load deployment history: ${err.message}`);
    } finally {
      loading = false;
    }
  }

  function handleSelectRun(e: CustomEvent<number>) {
    selectedRunId = e.detail;
  }

  function handleBackToHistory() {
    selectedRunId = null;
  }

  function handleOpenNewModal(
    e?: CustomEvent<{ projectName?: string; connectionName?: string } | void>
  ) {
    if (e?.detail) {
      if (e.detail.projectName) modalProject = e.detail.projectName;
      if (e.detail.connectionName) modalConnection = e.detail.connectionName;
    }
    showNewModal = true;
  }

  async function handleDeploymentStarted(
    e: CustomEvent<{ projectName: string; connectionName: string }>
  ) {
    $selectedProjectName = e.detail.projectName;
    $selectedConnectionName = e.detail.connectionName;

    // Refresh history so the new run record is fetched
    await loadHistory();

    // Select the new run or active run
    if ($activeDeploymentId) {
      selectedRunId = $activeDeploymentId;
    } else if ($deploymentHistory.length > 0) {
      selectedRunId = $deploymentHistory[0].id;
    }
  }
</script>

<div class="deployments-view-container">
  {#if selectedDeployment !== null}
    <!-- Dedicated Deployment Details View -->
    <DeploymentDetail
      deployment={selectedDeployment}
      allDeployments={$deploymentHistory}
      projects={$projects}
      connections={$connections}
      bind:activeRollbackId
      bind:failedRollbackIds
      on:back={handleBackToHistory}
      on:newDeployment={handleOpenNewModal}
      on:refresh={loadHistory}
    />
  {:else}
    <!-- Default Scannable Deployment History View -->
    <DeploymentHistory
      deployments={$deploymentHistory}
      projects={$projects}
      connections={$connections}
      {loading}
      bind:activeRollbackId
      bind:failedRollbackIds
      on:selectRun={handleSelectRun}
      on:newDeployment={handleOpenNewModal}
      on:refresh={loadHistory}
    />
  {/if}

  <!-- New Deployment Modal Workflow -->
  {#if showNewModal}
    <NewDeploymentModal
      projects={$projects}
      connections={$connections}
      initialProjectName={modalProject}
      initialConnectionName={modalConnection}
      on:close={() => (showNewModal = false)}
      on:deployed={handleDeploymentStarted}
    />
  {/if}
</div>

<style>
  .deployments-view-container {
    height: 100%;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }
</style>
