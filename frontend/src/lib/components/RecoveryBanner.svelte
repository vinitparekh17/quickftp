<script lang="ts">
  import Icon from './Icon.svelte';
  import { onMount } from 'svelte';
  import { pendingDeployments, currentPage, addToast } from '../stores';
  import {
    getPendingDeployments,
    rollbackDeployment,
    getConnectionNameById,
  } from '../wailsjs';

  let rollingBack = false;

  onMount(async () => {
    try {
      $pendingDeployments = await getPendingDeployments();
    } catch (err: any) {
      console.warn('Could not query pending deployments:', err);
    }
  });

  async function handleRollbackFirst() {
    if ($pendingDeployments.length === 0) return;
    const dep = $pendingDeployments[0];
    const connName = getConnectionNameById(dep.connectionId);
    if (
      !confirm(
        `Interrupted deployment #${dep.id} detected from previous run. Do you want to roll back changes using the verified remote backup?`
      )
    ) {
      return;
    }

    rollingBack = true;
    try {
      await rollbackDeployment(dep.id, connName);
      addToast('success', `Pending deployment #${dep.id} rolled back`);
      $pendingDeployments = await getPendingDeployments();
    } catch (err: any) {
      addToast('error', `Rollback failed: ${err.message}`);
    } finally {
      rollingBack = false;
    }
  }

  function viewHistory() {
    $currentPage = 'history';
  }
</script>

{#if $pendingDeployments.length > 0}
  <div class="recovery-banner">
    <div class="recovery-banner-text">
      <Icon name="warning" size={18} />
      <div>
        <strong>Startup Recovery:</strong>
        {$pendingDeployments.length} unfinished deployment(s) detected from previous
        run.
      </div>
    </div>
    <div style="display: flex; gap: 8px;">
      <button class="btn btn-secondary btn-sm" on:click={viewHistory}>
        <Icon name="history" size={14} />
        View in History
      </button>
      <button
        class="btn btn-danger btn-sm"
        on:click={handleRollbackFirst}
        disabled={rollingBack}
      >
        <Icon name="rollback" size={14} />
        {rollingBack
          ? 'Rolling back...'
          : `Rollback #${$pendingDeployments[0].id}`}
      </button>
    </div>
  </div>
{/if}
