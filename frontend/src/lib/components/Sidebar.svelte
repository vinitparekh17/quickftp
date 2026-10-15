<script lang="ts">
  import { currentPage, pendingDeployments } from '../stores';
  import type { Page } from '../types';

  function setPage(page: Page) {
    $currentPage = page;
  }
</script>

<aside class="app-sidebar">
  <div class="sidebar-brand">
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
      <path d="M4 14.899A7 7 0 1 1 15.71 8h1.79a4.5 4.5 0 0 1 2.5 8.242" />
      <path d="m8 17 4-4 4 4" />
      <path d="M12 13v9" />
    </svg>
    <span>QuickFTP</span>
  </div>

  <nav class="sidebar-nav">
    <button class="sidebar-nav-item" class:active={$currentPage === 'dashboard'} on:click={() => setPage('dashboard')}>
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg><span>Dashboard</span>
    </button>
    <button
      class="sidebar-nav-item"
      class:active={$currentPage === 'connections'}
      on:click={() => setPage('connections')}
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <rect width="20" height="8" x="2" y="2" rx="2" ry="2" />
        <rect width="20" height="8" x="2" y="14" rx="2" ry="2" />
        <line x1="6" x2="6.01" y1="6" y2="6" />
        <line x1="6" x2="6.01" y1="18" y2="18" />
      </svg>
      <span>Connections</span>
    </button>
    <button
      class="sidebar-nav-item"
      class:active={$currentPage === 'projects'}
      on:click={() => setPage('projects')}
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 8 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z" />
      </svg>
      <span>Projects</span>
    </button>
    <button class="sidebar-nav-item" class:active={$currentPage === 'settings'} on:click={() => setPage('settings')}>
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="m19.4 15 .1.1 1.4 1.1-1.4 2.4-1.7-.7a8 8 0 0 1-1.8 1l-.3 1.8h-2.8l-.3-1.8a8 8 0 0 1-1.8-1l-1.7.7-1.7.7-1.4-2.4 1.4-1.1a7 7 0 0 1 0-2l-1.4-1.1 1.4-2.4 1.7.7a8 8 0 0 1 1.8-1l.3-1.8h2.8l.3 1.8a8 8 0 0 1 1.8 1l1.7-.7 1.4 2.4-1.4 1.1a7 7 0 0 1 0 2z"/></svg><span>Settings</span>
    </button>

    <button
      class="sidebar-nav-item"
      class:active={$currentPage === 'deploy'}
      on:click={() => setPage('deploy')}
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <polygon points="5 3 19 12 5 21 5 3" />
      </svg>
      <span>Deploy</span>
    </button>

    <button
      class="sidebar-nav-item"
      class:active={$currentPage === 'history'}
      on:click={() => setPage('history')}
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
        <path d="M3 3v5h5" />
        <path d="M12 7v5l4 2" />
      </svg>
      <span>History</span>
      {#if $pendingDeployments.length > 0}
        <span class="badge badge-warning" style="margin-left: auto; font-size: 10px;">
          {$pendingDeployments.length}
        </span>
      {/if}
    </button>
  </nav>

  <div class="sidebar-footer">
    QuickFTP v0.1.0 • Desktop
  </div>
</aside>

