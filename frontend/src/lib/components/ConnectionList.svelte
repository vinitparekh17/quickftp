<script lang="ts">
  import Icon from './Icon.svelte';
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
    if (
      !confirm(
        `Are you sure you want to delete connection "${c.name}"? This will also remove stored credentials.`
      )
    ) {
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

<div class="card page-card">
  <div class="card-header">
    <div>
      <h2>FTP Connections</h2>
      <p class="text-sm text-muted">
        Manage remote FTP and explicit FTPS server credentials
      </p>
    </div>
    <button class="btn btn-primary" on:click={handleAdd}>
      <Icon name="plus" size={16} />
      <span>Add Connection</span>
    </button>
  </div>

  <div class="card-body" style="padding: 0;">
    {#if loading}
      <div style="padding: 32px; text-align: center;" class="text-muted">
        Loading connections...
      </div>
    {:else if $connections.length === 0}
      <div class="empty-state">
        <Icon name="server" size={24} />
        <h3>No connections yet</h3>
        <p>Create an FTP or FTPS connection to deploy your build artifacts.</p>
        <button class="btn btn-primary" on:click={handleAdd}
          >Add First Connection</button
        >
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
                <td
                  >{#if connectionState[c.id] === 'checking'}<span
                      class="activity-status running"><i></i>Testing</span
                    >{:else if connectionState[c.id] === 'connected'}<span
                      class="activity-status success"><i></i>Connected</span
                    >{:else if connectionState[c.id] === 'failed'}<span
                      class="activity-status failed"><i></i>Failed</span
                    >{:else}<span class="activity-status"
                      ><i></i>Not tested</span
                    >{/if}</td
                >
                <td style="text-align: right; white-space: nowrap;">
                  <div class="table-actions">
                    <button
                      class="table-icon-action action-primary"
                      on:click={() => handleTest(c)}
                      disabled={testingId === c.id}
                      title={testingId === c.id ? `Testing connection "${c.name}"...` : `Test connection "${c.name}"`}
                      aria-label={`Test connection "${c.name}"`}
                    >
                      <Icon name="refresh" size={16} spin={testingId === c.id} />
                    </button>
                    <button
                      class="table-icon-action action-secondary"
                      on:click={() => handleEdit(c)}
                      title={`Edit connection "${c.name}"`}
                      aria-label={`Edit connection "${c.name}"`}
                    >
                      <Icon name="edit" size={16} />
                    </button>
                    <button
                      class="table-icon-action action-danger"
                      on:click={() => handleDelete(c)}
                      title={`Delete connection "${c.name}"`}
                      aria-label={`Delete connection "${c.name}"`}
                    >
                      <Icon name="trash" size={16} />
                    </button>
                  </div>
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
