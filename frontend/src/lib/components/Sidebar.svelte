<script lang="ts">
  import Icon from './Icon.svelte';
  import { currentPage, pendingDeployments } from '../stores';
  import type { Page } from '../types';
  import appIcon from '../../assets/quickftp-icon.png';

  function setPage(page: Page) {
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
      class:active={$currentPage === 'projects'}
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
      class:active={$currentPage === 'history' || $currentPage === 'deploy'}
      on:click={() => setPage('history')}
    >
      <Icon name="deploy" size={18} />
      <span>Deployments</span>
      {#if $pendingDeployments.length > 0}
        <span
          class="badge badge-warning"
          style="margin-left: auto; font-size: 10px;"
        >
          {$pendingDeployments.length}
        </span>
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

  <div class="sidebar-footer">QuickFTP v0.1.0 • Desktop</div>
</aside>
