<script lang="ts">
  import { onMount } from 'svelte';
  import { currentPage, toasts, deployLogs, deployPhase, deployStatus, activeDeploymentId, addToast } from './lib/stores';
  import { onEvent } from './lib/wailsjs';
  import Sidebar from './lib/components/Sidebar.svelte';
  import ConnectionList from './lib/components/ConnectionList.svelte';
  import ProjectList from './lib/components/ProjectList.svelte';
  import DeployView from './lib/components/DeployView.svelte';
  import HistoryView from './lib/components/HistoryView.svelte';
  import RecoveryBanner from './lib/components/RecoveryBanner.svelte';
  import Dashboard from './lib/components/Dashboard.svelte';
  import SettingsView from './lib/components/SettingsView.svelte';

  onMount(() => {
    // Listen for backend Wails events
    const unsubLog = onEvent('deploy:log', (data: { stream: string; line: string }) => {
      const stamp = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
      deployLogs.update(logs => [...logs, { ...data, line: `${stamp}  ${data.line}` }]);
    });

    const unsubStarted = onEvent('deploy:started', (data: { project: string; connection: string }) => {
      deployStatus.set('running');
      deployPhase.set('planning');
    });

    const unsubPhase = onEvent('deploy:phase', (data: { deploymentId: number; phase: string }) => {
      deployPhase.set(data.phase);
    });

    const unsubComplete = onEvent('deploy:complete', (data: { deploymentId: number }) => {
      deployStatus.set('completed');
      deployPhase.set('done');
      activeDeploymentId.set(data.deploymentId);
      addToast('success', `Deployment #${data.deploymentId} completed successfully!`);
    });

    const unsubError = onEvent('deploy:error', (data: { deploymentId: number; error: string }) => {
      deployStatus.set('failed');
      activeDeploymentId.set(data.deploymentId);
      addToast('error', `Deployment #${data.deploymentId} failed: ${data.error}`);
    });

    const unsubCancelled = onEvent('deploy:cancelled', (data: { deploymentId: number; error: string }) => {
      deployStatus.set('cancelled');
      activeDeploymentId.set(data.deploymentId || null);
      addToast('info', data.deploymentId ? `Deployment #${data.deploymentId} cancelled. Use History to review or roll back.` : 'Deployment cancelled before the remote transfer began.');
    });

    return () => {
      unsubLog();
      unsubStarted();
      unsubPhase();
      unsubComplete();
      unsubError();
      unsubCancelled();
    };
  });

</script>

<div class="app-layout">
  <Sidebar />

  <div class="app-main">
    <main class="app-content">
      <RecoveryBanner />

      {#if $currentPage === 'dashboard'}
        <Dashboard />
      {:else if $currentPage === 'connections'}
        <ConnectionList />
      {:else if $currentPage === 'projects'}
        <ProjectList />
      {:else if $currentPage === 'deploy'}
        <DeployView />
      {:else if $currentPage === 'history'}
        <HistoryView />
      {:else if $currentPage === 'settings'}
        <SettingsView />
      {/if}
    </main>
  </div>
</div>

<!-- Toast notifications -->
{#each $toasts as t (t.id)}
  <div class="toast toast-{t.type}">
    {#if t.type === 'success'}
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <polyline points="20 6 9 17 4 12" />
      </svg>
    {:else if t.type === 'error'}
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="12" cy="12" r="10" />
        <line x1="15" y1="9" x2="9" y2="15" />
        <line x1="9" y1="9" x2="15" y2="15" />
      </svg>
    {:else}
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="12" cy="12" r="10" />
        <line x1="12" y1="16" x2="12" y2="12" />
        <line x1="12" y1="8" x2="12.01" y2="8" />
      </svg>
    {/if}
    <span>{t.message}</span>
  </div>
{/each}

