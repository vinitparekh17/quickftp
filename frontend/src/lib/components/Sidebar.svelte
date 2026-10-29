<script lang="ts">
  import Icon from './Icon.svelte';
  import {
    currentPage,
    pendingDeployments,
    transferSummary,
    deployStatus,
    requestedDeploymentId,
    activeDeployment,
    deploymentProjectId,
  } from '../stores';
  import type { Page } from '../types';
  import appIcon from '../../assets/quickftp-icon.png';

  function setPage(page: Page) {
    if (page === 'history' && $deployStatus === 'running')
      $requestedDeploymentId = 0;
    $currentPage = page;
  }
</script>

<aside class="app-sidebar">
  <div class="sidebar-brand">
    <img src={appIcon} alt="QuickFTP" class="brand-logo" />
    <span>QuickFTP</span>
  </div>

  <nav class="sidebar-nav">
    <button
      class="sidebar-nav-item"
      class:active={$currentPage === 'dashboard'}
      on:click={() => setPage('dashboard')}
    >
      <Icon name="dashboard" size={18} />
      <span>Dashboard</span>
    </button>

    <button
      class="sidebar-nav-item"
      class:active={$currentPage === 'files'}
      on:click={() => setPage('files')}
    >
      <Icon name="folderTree" size={18} />
      <span>Files</span>
    </button>

    <button
      class="sidebar-nav-item"
      class:active={['projects', 'history', 'deploy'].includes($currentPage)}
      on:click={() => setPage('projects')}
    >
      <Icon name="projects" size={18} />
      <span>Projects</span>
    </button>

    <button
      class="sidebar-nav-item"
      class:active={$currentPage === 'connections'}
      on:click={() => setPage('connections')}
    >
      <Icon name="server" size={18} />
      <span>Connections</span>
    </button>

    <button
      class="sidebar-nav-item"
      class:active={$currentPage === 'activity'}
      on:click={() => setPage('activity')}
    >
      <Icon name="transfer" size={18} />
      <span>Activity</span>
      {#if $transferSummary.attention || $transferSummary.active}
        <span
          class="sidebar-count"
          class:needs-attention={$transferSummary.attention > 0}
          title={`${$transferSummary.activeBatches} active transfer batches; ${$transferSummary.attentionBatches} batches with ${$transferSummary.attention} files needing attention`}
          >{$transferSummary.attentionBatches ||
            $transferSummary.activeBatches}</span
        >
      {/if}
    </button>

    <button
      class="sidebar-nav-item"
      class:active={$currentPage === 'settings'}
      on:click={() => setPage('settings')}
    >
      <Icon name="settings" size={18} />
      <span>Settings</span>
    </button>
  </nav>

  {#if $deployStatus === 'running'}<button
      class="sidebar-nav-item"
      on:click={() => {
        $deploymentProjectId = $activeDeployment?.projectId || null;
        $requestedDeploymentId = 0;
        $currentPage = 'history';
      }}><Icon name="loader" size={15} spin /> Live deployment</button
    >{/if}

  <div class="sidebar-footer">QuickFTP v0.1.0 • Desktop</div>
</aside>
