<script lang="ts">
  import { onMount } from 'svelte';
  import { selectDeployment } from '../deploymentSelection';
  import {
    deploymentHistory,
    connections,
    projects,
    deployStatus,
    activeDeployment,
    selectedProjectName,
    selectedConnectionName,
    currentPage,
    requestedDeploymentId,
    deploymentProjectId,
    addToast,
  } from '../stores';
  import {
    getDeploymentHistory,
    getProjectDeployments,
    getDeployment,
    getConnections,
    getProjects,
  } from '../wailsjs';
  import DeploymentHistory from './deploy/DeploymentHistory.svelte';
  import DeploymentDetail from './deploy/DeploymentDetail.svelte';
  import NewDeploymentModal from './deploy/NewDeploymentModal.svelte';

  // Navigation state within Deployments:
  // selectedRunId === null -> History View (default scannable table)
  // selectedRunId !== null -> Detail View for that specific execution run
  let selectedRunId: number | null = $deployStatus === 'running' ? 0 : null;

  // New Deployment Modal state
  let showNewModal = false;
  let modalProject = '';
  let modalConnection = '';

  let loading = true;
  let activeRollbackId: number | null = null;
  let failedRollbackIds: Set<number> = new Set();

  $: selectedDeployment = selectDeployment(
    selectedRunId,
    $deploymentHistory,
    $activeDeployment
  );

  $: scopedProjectId = selectedDeployment?.projectId || $deploymentProjectId;
  $: scopedProject = $projects.find(
    (project) => project.id === scopedProjectId
  );
  $: projectRuns = $deploymentHistory.filter(
    (run) => run.projectId === scopedProjectId
  );
  $: if (selectedDeployment?.projectId)
    $deploymentProjectId = selectedDeployment.projectId;
  $: if (scopedProject && !showNewModal) modalProject = scopedProject.name;
  let fetchedProjectId: number | null = null;
  $: if (
    $currentPage !== 'deploy' &&
    !showNewModal &&
    scopedProjectId &&
    scopedProjectId !== fetchedProjectId
  ) {
    fetchedProjectId = scopedProjectId;
    void loadProjectHistory(scopedProjectId);
  }
  async function loadProjectHistory(id: number) {
    try {
      const scoped = await getProjectDeployments(id);
      const all = new Map($deploymentHistory.map((run) => [run.id, run]));
      for (const run of scoped) all.set(run.id, run);
      $deploymentHistory = [...all.values()].sort((a, b) => b.id - a.id);
    } catch (error: any) {
      addToast('error', `Could not load project deployments: ${error.message}`);
    }
  }
  let observedStatus = $deployStatus;
  $: if ($deployStatus !== observedStatus) {
    observedStatus = $deployStatus;
    if ($deployStatus !== 'running' && $deployStatus !== 'idle')
      void loadHistory();
  }

  $: if ($requestedDeploymentId !== null) {
    selectedRunId = $requestedDeploymentId;
    $requestedDeploymentId = null;
  }

  // Pre-fill modal if requested from Dashboard / ProjectList
  $: if ($selectedProjectName && !modalProject) {
    modalProject = $selectedProjectName;
  }
  $: if ($selectedConnectionName && !modalConnection) {
    modalConnection = $selectedConnectionName;
  }

  // If user navigated directly via "Deploy" button elsewhere ($currentPage === 'deploy')
  $: if ($currentPage === 'deploy') {
    const requestedProject = $projects.find(
      (project) => project.name === $selectedProjectName
    );
    if (requestedProject) $deploymentProjectId = requestedProject.id;
    showNewModal = true;
    $currentPage = 'history';
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

    if (!showNewModal) await loadHistory();
  });

  async function loadHistory() {
    loading = true;
    try {
      const recent = await getDeploymentHistory(50);
      const all = new Map($deploymentHistory.map((run) => [run.id, run]));
      for (const run of recent) all.set(run.id, run);
      if (selectedRunId && selectedRunId > 0 && !all.has(selectedRunId)) {
        const requested = await getDeployment(selectedRunId);
        if (requested) all.set(requested.id, requested);
      }
      $deploymentHistory = [...all.values()].sort((a, b) => b.id - a.id);
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

  function handleDeploymentStarted(
    e: CustomEvent<{ projectName: string; connectionName: string }>
  ) {
    $deploymentProjectId =
      $projects.find((project) => project.name === e.detail.projectName)?.id ??
      null;
    $selectedProjectName = e.detail.projectName;
    $selectedConnectionName = e.detail.connectionName;
    selectedRunId = 0;
    void loadHistory();
  }
</script>

<div class="deployments-view-container">
  <nav class="hierarchy-breadcrumb" aria-label="Project navigation">
    <button class="text-button" on:click={() => ($currentPage = 'projects')}
      >Projects</button
    ><span>/</span>{#if scopedProject}<button
        class="text-button"
        on:click={() => {
          $deploymentProjectId = scopedProject.id;
          selectedRunId = null;
        }}>{scopedProject.name}</button
      ><span>/</span><span>Deployments</span>{:else}<span>Select a project</span
      >{/if}
  </nav>
  {#if selectedDeployment !== null}
    <!-- Dedicated Deployment Details View -->
    {#key `${selectedRunId}:${selectedDeployment.id}`}
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
    {/key}
  {:else if !scopedProjectId}
    <div class="card activity-groups">
      {#each $projects as project}<button
          class="activity-group-card"
          on:click={() => {
            $deploymentProjectId = project.id;
            $selectedProjectName = project.name;
          }}
          ><strong>{project.name}</strong><span class="cell-secondary"
            >View deployments and logs for this project</span
          ></button
        >{/each}
    </div>
  {:else}
    <!-- Default Scannable Deployment History View -->
    <DeploymentHistory
      deployments={projectRuns}
      projectName={scopedProject?.name || ''}
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
