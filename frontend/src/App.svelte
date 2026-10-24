<script lang="ts">
  import Icon from './lib/components/Icon.svelte';
  import { onMount } from 'svelte';
  import {
    currentPage,
    toasts,
    deployLogs,
    deployPhase,
    deployStatus,
    activeDeploymentId,
    transfers,
    activityLogs,
    addToast,
  } from './lib/stores';
  import { onEvent } from './lib/wailsjs';
  import Sidebar from './lib/components/Sidebar.svelte';
  import ConnectionList from './lib/components/ConnectionList.svelte';
  import ProjectList from './lib/components/ProjectList.svelte';
  import DeployView from './lib/components/DeployView.svelte';
  import RecoveryBanner from './lib/components/RecoveryBanner.svelte';
  import Dashboard from './lib/components/Dashboard.svelte';
  import SettingsView from './lib/components/SettingsView.svelte';
  import FilesView from './lib/components/FilesView.svelte';
  import TransferPanel from './lib/components/TransferPanel.svelte';

  onMount(() => {
    // Listen for backend Wails events
    const unsubLog = onEvent(
      'deploy:log',
      (data: { stream: string; line: string }) => {
        const line = String(data?.line ?? '');
        const stamp = new Date().toLocaleTimeString([], {
          hour: '2-digit',
          minute: '2-digit',
          second: '2-digit',
        });
        deployLogs.update((logs) => [
          ...logs,
          { ...data, line: `${stamp}  ${line}` },
        ]);

        // Keep the visible pipeline aligned with transfer logs if phase events
        // arrive late or are missed by the frontend event bridge.
        if (line.startsWith('Uploading ')) deployPhase.set('upload');
        else if (line.startsWith('Upload verified '))
          deployPhase.set('verify_upload');
        else if (line.startsWith('Backup verified '))
          deployPhase.set('verify_backup');
        else if (line.startsWith('Removing stale '))
          deployPhase.set('delete');
      }
    );

    const unsubStarted = onEvent(
      'deploy:started',
      (data: { project: string; connection: string }) => {
        deployStatus.set('running');
      }
    );

    const unsubPhase = onEvent(
      'deploy:phase',
      (data: { deploymentId: number; phase: string }) => {
        deployPhase.set(data.phase);
      }
    );

    const unsubComplete = onEvent(
      'deploy:complete',
      (data: { deploymentId: number }) => {
        deployStatus.set('completed');
        deployPhase.set('done');
        activeDeploymentId.set(data.deploymentId);
        addToast(
          'success',
          `Deployment #${data.deploymentId} completed successfully!`
        );
      }
    );

    const unsubError = onEvent(
      'deploy:error',
      (data: { deploymentId: number; error: string }) => {
        deployStatus.set('failed');
        activeDeploymentId.set(data.deploymentId);
        const stamp = new Date().toLocaleTimeString([], {
          hour: '2-digit',
          minute: '2-digit',
          second: '2-digit',
        });
        deployLogs.update((logs) => [
          ...logs,
          {
            stream: 'stderr',
            line: `${stamp}  Deployment #${data.deploymentId} failed: ${data.error}`,
          },
        ]);
        addToast(
          'error',
          `Deployment #${data.deploymentId} failed: ${data.error}`
        );
      }
    );

    const unsubCancelled = onEvent(
      'deploy:cancelled',
      (data: { deploymentId: number; error: string }) => {
        deployStatus.set('cancelled');
        activeDeploymentId.set(data.deploymentId || null);
        addToast(
          'info',
          data.deploymentId
            ? `Deployment #${data.deploymentId} cancelled. Use History to review or roll back.`
            : 'Deployment cancelled before the remote transfer began.'
        );
      }
    );
    const unsubTransfer = onEvent('transfer:changed', (data: any) => {
      if (!data?.id) return;
      transfers.update((xs) => {
        const current = Array.isArray(xs) ? xs : [];
        const i = current.findIndex((x) => x.id === data.id);
        if (i < 0) return [data, ...current];
        const next = [...current];
        next[i] = data;
        return next;
      });
    });
    const unsubDeployPhase = onEvent('deploy:phase', (data: any) => {
      if (data?.phase)
        activityLogs.update((xs) => [
          ...xs.slice(-999),
          {
            timestamp: new Date().toISOString(),
            level: 'INFO',
            category: 'Deployment',
            message: `Deployment phase: ${data.phase}`,
          },
        ]);
    });
    const unsubDeployResult = onEvent('deploy:complete', (data: any) =>
      activityLogs.update((xs) => [
        ...xs.slice(-999),
        {
          timestamp: new Date().toISOString(),
          level: 'SUCCESS',
          category: 'Deployment',
          message: `Deployment #${data.deploymentId} completed`,
        },
      ])
    );
    const unsubDeployError = onEvent('deploy:error', (data: any) =>
      activityLogs.update((xs) => [
        ...xs.slice(-999),
        {
          timestamp: new Date().toISOString(),
          level: 'ERROR',
          category: 'Deployment',
          message: `Deployment #${data.deploymentId} failed: ${data.error}`,
        },
      ])
    );
    const unsubTransferActivity = onEvent('transfer:log', (data: any) => {
      if (data?.message) {
        const line = String(data.message);
        const upper = line.toUpperCase();
        const category = upper.startsWith('BUILD ')
          ? 'Build'
          : upper.includes('BACKUP') || upper.includes('RESTORE')
            ? 'Backup'
            : upper.startsWith('DEPLOYMENT ')
              ? 'Deployment'
              : 'Transfer';
        activityLogs.update((xs) => [
          ...xs.slice(-999),
          { ...data, category, message: line.replace(/^(BUILD|DEPLOYMENT) /, '') },
        ]);
      }
    });

    return () => {
      unsubLog();
      unsubStarted();
      unsubPhase();
      unsubComplete();
      unsubError();
      unsubCancelled();
      unsubTransfer();
      unsubDeployPhase();
      unsubDeployResult();
      unsubDeployError();
      unsubTransferActivity();
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


      {:else if $currentPage === "history" || $currentPage === "deploy"}
        <DeployView />
      {:else if $currentPage === 'settings'}
        <SettingsView />
      {:else if $currentPage === 'files'}
        <FilesView />
      {/if}
    </main>
    <TransferPanel />
  </div>
</div>

<!-- Toast notifications -->
{#each $toasts as t (t.id)}
  <div class="toast toast-{t.type}">
    {#if t.type === 'success'}
      <Icon name="success" size={18} />
    {:else if t.type === 'error'}
      <Icon name="error" size={18} />
    {:else}
      <Icon name="info" size={18} />
    {/if}
    <span>{t.message}</span>
  </div>
{/each}


