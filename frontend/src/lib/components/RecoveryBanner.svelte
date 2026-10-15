<script lang="ts">
  import { onMount } from 'svelte';
  import { pendingDeployments, currentPage, addToast } from '../stores';
  import { getPendingDeployments, rollbackDeployment, getConnectionNameById } from '../wailsjs';

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
    if (!confirm(`Interrupted deployment #${dep.id} detected from previous run. Do you want to roll back changes using the verified remote backup?`)) {
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
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z" />
        <line x1="12" y1="9" x2="12" y2="13" />
        <line x1="12" y1="17" x2="12.01" y2="17" />
      </svg>
      <div>
        <strong>Startup Recovery:</strong>
        {$pendingDeployments.length} unfinished deployment(s) detected from previous run.
      </div>
    </div>
    <div style="display: flex; gap: 8px;">
      <button class="btn btn-secondary btn-sm" on:click={viewHistory}>
        View in History
      </button>
      <button class="btn btn-danger btn-sm" on:click={handleRollbackFirst} disabled={rollingBack}>
        {rollingBack ? 'Rolling back...' : `Rollback #${$pendingDeployments[0].id}`}
      </button>
    </div>
  </div>
{/if}

