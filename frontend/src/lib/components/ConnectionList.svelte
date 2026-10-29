<script lang="ts">
  import Icon from './Icon.svelte';
  import { onMount } from 'svelte';
  import type { ConnectionDTO } from '../types';
  import {
    connections,
    addToast,
    currentPage,
    activityConnectionId,
    activityTab,
  } from '../stores';
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

<div class="view-full-height">
  <section class="page-heading">
    <div>
      <h1>Connections</h1>
      <p class="page-subtitle">
        Saved servers, credentials, and connection tests.
      </p>
    </div>
    <button class="btn btn-primary" on:click={handleAdd}
      ><Icon name="plus" size={15} /> Add connection</button
    >
  </section>
  <div class="card page-card resource-list-card">
    {#if loading}<div class="files-state">
        <Icon name="loader" size={20} spin /> Loading connections…
      </div>
    {:else if !$connections.length}<div class="empty-state">
        <Icon name="server" size={28} />
        <h3>No connections yet</h3>
        <p>Add the FTP or FTPS server you want to publish to.</p>
        <button class="btn btn-primary" on:click={handleAdd}
          >Add connection</button
        >
      </div>
    {:else}<div class="table-wrap">
        <table class="resource-table">
          <thead
            ><tr
              ><th>Connection</th><th>Server &amp; remote folder</th><th
                >Last test</th
              ><th class="row-actions-heading">Actions</th></tr
            ></thead
          ><tbody>
            {#each $connections as c (c.id)}
              <tr
                ><td
                  ><div class="resource-name">
                    <strong>{c.name}</strong><span
                      class="badge"
                      class:badge-info={c.protocol === 'ftps_explicit'}
                      >{c.protocol === 'ftps_explicit' ? 'FTPS' : 'FTP'}</span
                    >
                  </div>
                  <span class="cell-secondary">{c.username}</span></td
                ><td
                  ><span class="text-mono resource-path">{c.host}:{c.port}</span
                  ><span class="cell-secondary text-mono resource-path"
                    >{c.remoteRoot}</span
                  ></td
                >
                <td
                  >{#if connectionState[c.id] === 'checking'}<span
                      class="activity-status"
                      ><Icon name="loader" size={13} spin /> Testing…</span
                    >{:else if connectionState[c.id] === 'connected'}<span
                      class="activity-status success"
                      ><Icon name="success" size={13} /> Passed</span
                    >{:else if connectionState[c.id] === 'failed'}<span
                      class="activity-status failed"
                      ><Icon name="error" size={13} /> Failed</span
                    >{:else}<span class="text-muted">Not tested</span>{/if}</td
                >
                <td
                  ><div class="table-actions">
                    <button
                      class="btn btn-secondary btn-sm"
                      on:click={() => {
                        $activityConnectionId = c.id;
                        $activityTab = 'Transfers';
                        $currentPage = 'activity';
                      }}>Transfers</button
                    >
                    <button
                      class="btn btn-secondary btn-sm"
                      on:click={() => handleTest(c)}
                      disabled={testingId !== null}
                      ><Icon
                        name="refresh"
                        size={13}
                        spin={testingId === c.id}
                      /> Test</button
                    ><button
                      class="table-icon-action action-secondary"
                      on:click={() => handleEdit(c)}
                      title={`Edit ${c.name}`}
                      aria-label={`Edit ${c.name}`}
                      ><Icon name="edit" size={16} /></button
                    ><button
                      class="table-icon-action action-danger"
                      on:click={() => handleDelete(c)}
                      title={`Delete ${c.name}`}
                      aria-label={`Delete ${c.name}`}
                      ><Icon name="trash" size={16} /></button
                    >
                  </div></td
                ></tr
              >
            {/each}
          </tbody>
        </table>
      </div>{/if}
  </div>
</div>
{#if showForm}<ConnectionForm
    connection={editingConnection}
    on:saved={handleSaved}
    on:cancel={handleCancel}
  />{/if}
