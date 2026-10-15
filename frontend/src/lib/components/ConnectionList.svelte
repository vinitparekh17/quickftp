<script lang="ts">
  import { onMount } from 'svelte';
  import type { ConnectionDTO } from '../types';
  import { connections, addToast } from '../stores';
  import { getConnections, deleteConnection, testConnection } from '../wailsjs';
  import ConnectionForm from './ConnectionForm.svelte';

  let showForm = false;
  let editingConnection: ConnectionDTO | null = null;
  let testingId: number | null = null;
  let connectionState: Record<number, 'checking' | 'connected' | 'failed'> = {};
  let loading = true;

  onMount(async () => {
    await loadConnections();
  });

  async function loadConnections() {
    loading = true;
    try {
      $connections = await getConnections();
    } catch (err: any) {
      addToast('error', `Failed to load connections: ${err.message}`);
    } finally {
      loading = false;
    }
  }

  function handleAdd() {
    editingConnection = null;
    showForm = true;
  }

  function handleEdit(c: ConnectionDTO) {
    editingConnection = { ...c };
    showForm = true;
  }

  async function handleDelete(c: ConnectionDTO) {
    if (!confirm(`Are you sure you want to delete connection "${c.name}"? This will also remove stored credentials.`)) {
      return;
    }
    try {
      await deleteConnection(c.id);
      addToast('success', `Connection "${c.name}" deleted`);
      await loadConnections();
    } catch (err: any) {
      addToast('error', `Failed to delete connection: ${err.message}`);
    }
  }

  async function handleTest(c: ConnectionDTO) {
    testingId = c.id;
    connectionState = { ...connectionState, [c.id]: 'checking' };
    try {
      await testConnection(c);
      connectionState = { ...connectionState, [c.id]: 'connected' };
      addToast('success', `Connection "${c.name}" tested successfully`);
    } catch (err: any) {
      connectionState = { ...connectionState, [c.id]: 'failed' };
      addToast('error', `Connection "${c.name}" test failed: ${err.message}`);
    } finally {
      testingId = null;
    }
  }

  function handleSaved() {
    showForm = false;
    editingConnection = null;
    loadConnections();
  }

  function handleCancel() {
    showForm = false;
    editingConnection = null;
  }
</script>

<div class="card">
  <div class="card-header">
    <div>
      <h2>FTP Connections</h2>
      <p class="text-sm text-muted">Manage remote FTP and explicit FTPS server credentials</p>
    </div>
    <button class="btn btn-primary" on:click={handleAdd}>
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <line x1="12" y1="5" x2="12" y2="19" />
        <line x1="5" y1="12" x2="19" y2="12" />
      </svg>
      <span>Add Connection</span>
    </button>
  </div>

  <div class="card-body" style="padding: 0;">
    {#if loading}
      <div style="padding: 32px; text-align: center;" class="text-muted">Loading connections...</div>
    {:else if $connections.length === 0}
      <div class="empty-state">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <rect width="20" height="8" x="2" y="2" rx="2" ry="2" />
          <rect width="20" height="8" x="2" y="14" rx="2" ry="2" />
          <line x1="6" x2="6.01" y1="6" y2="6" />
          <line x1="6" x2="6.01" y1="18" y2="18" />
        </svg>
        <h3>No connections yet</h3>
        <p>Create an FTP or FTPS connection to deploy your build artifacts.</p>
        <button class="btn btn-primary" on:click={handleAdd}>Add First Connection</button>
      </div>
    {:else}
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Protocol</th>
              <th>Host & Port</th>
              <th>Username</th>
              <th>Remote Root</th>
              <th>Status</th>
              <th style="text-align: right;">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each $connections as c (c.id)}
              <tr>
                <td>
                  <strong>{c.name}</strong>
                </td>
                <td>
                  {#if c.protocol === 'ftps_explicit'}
                    <span class="badge badge-info">FTPS</span>
                  {:else}
                    <span class="badge badge-gray">FTP</span>
                  {/if}
                </td>
                <td class="text-mono">{c.host}:{c.port}</td>
                <td>{c.username}</td>
                <td class="text-mono text-muted">{c.remoteRoot}</td>
                <td>{#if connectionState[c.id] === 'checking'}<span class="activity-status running"><i></i>Testing</span>{:else if connectionState[c.id] === 'connected'}<span class="activity-status success"><i></i>Connected</span>{:else if connectionState[c.id] === 'failed'}<span class="activity-status failed"><i></i>Failed</span>{:else}<span class="activity-status"><i></i>Not tested</span>{/if}</td>
                <td style="text-align: right; white-space: nowrap;">
                  <button
                    class="btn btn-secondary btn-sm"
                    on:click={() => handleTest(c)}
                    disabled={testingId === c.id}
                    title="Test connection"
                  >
                    {testingId === c.id ? 'Testing...' : 'Test'}
                  </button>
                  <button
                    class="btn btn-ghost btn-sm"
                    on:click={() => handleEdit(c)}
                    title="Edit connection"
                  >
                    Edit
                  </button>
                  <button
                    class="btn btn-ghost btn-sm"
                    style="color: var(--color-danger);"
                    on:click={() => handleDelete(c)}
                    title="Delete connection"
                  >
                    Delete
                  </button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</div>

{#if showForm}
  <ConnectionForm
    connection={editingConnection}
    on:saved={handleSaved}
    on:cancel={handleCancel}
  />
{/if}

