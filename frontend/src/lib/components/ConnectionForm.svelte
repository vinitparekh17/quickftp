<script lang="ts">
  import Icon from './Icon.svelte';
  import { createEventDispatcher } from 'svelte';
  import type { ConnectionDTO } from '../types';
  import { testConnection, saveConnection } from '../wailsjs';
  import { addToast } from '../stores';

  export let connection: ConnectionDTO | null = null;

  const dispatch = createEventDispatcher();

  let name = connection ? connection.name : '';
  let host = connection ? connection.host : '';
  let port = connection ? connection.port : 21;
  let protocol = connection ? connection.protocol : 'ftps_explicit';
  let username = connection ? connection.username : '';
  let password = '';
  let remoteRoot = connection ? connection.remoteRoot : '/';
  let tlsVerify = connection ? connection.tlsVerify : true;

  let testing = false;
  let saving = false;
  let errorMsg = '';

  function normalizeEndpoint() {
    const value = host.trim();
    if (!/^ftps?:\/\//i.test(value)) return;

    try {
      const url = new URL(value);
      if (url.protocol !== 'ftp:' && url.protocol !== 'ftps:')
        throw new Error('Unsupported URL scheme');
      if (
        !url.hostname ||
        url.search ||
        url.hash ||
        url.username ||
        url.password
      )
        throw new Error('Invalid FTP URL');
      host = url.hostname;
      if (url.port) port = Number(url.port);
      else if (url.protocol === 'ftps:') port = 990;
      if (url.pathname !== '/') remoteRoot = decodeURIComponent(url.pathname);
      protocol = url.protocol === 'ftps:' ? 'ftps_explicit' : 'ftp';
    } catch {
      errorMsg =
        'Enter a server name or an FTP/FTPS URL without credentials or query parameters';
    }
  }

  async function handleTest() {
    errorMsg = '';
    normalizeEndpoint();
    if (errorMsg) return;
    if (!host || !username) {
      errorMsg = 'Host and username are required to test connection';
      return;
    }
    testing = true;
    errorMsg = '';
    try {
      await testConnection({
        id: connection ? connection.id : 0,
        name: name || 'Test',
        host,
        port,
        protocol,
        username,
        password,
        remoteRoot,
        tlsVerify,
        createdAt: '',
      });
      addToast('success', 'Connection test succeeded! Remote host verified.');
    } catch (err: any) {
      errorMsg = err.message || 'Connection test failed';
      addToast('error', `Test failed: ${errorMsg}`);
    } finally {
      testing = false;
    }
  }

  async function handleSave() {
    errorMsg = '';
    normalizeEndpoint();
    if (errorMsg) return;
    if (!name.trim()) {
      errorMsg = 'Name is required';
      return;
    }
    if (!host.trim()) {
      errorMsg = 'Host is required';
      return;
    }
    if (!username.trim()) {
      errorMsg = 'Username is required';
      return;
    }
    if (!connection && !password) {
      errorMsg = 'Password is required for new connection';
      return;
    }

    saving = true;
    errorMsg = '';
    try {
      await saveConnection({
        id: connection ? connection.id : 0,
        name: name.trim(),
        host: host.trim(),
        port: Number(port) || 21,
        protocol,
        username: username.trim(),
        password,
        remoteRoot: remoteRoot.trim() || '/',
        tlsVerify,
        createdAt: connection ? connection.createdAt : '',
      });
      addToast('success', `Connection "${name}" saved successfully`);
      dispatch('saved');
    } catch (err: any) {
      errorMsg = err.message || 'Failed to save connection';
      addToast('error', errorMsg);
    } finally {
      saving = false;
    }
  }

  function handleCancel() {
    dispatch('cancel');
  }
</script>

<div class="modal-overlay">
  <div class="modal">
    <div class="modal-header">
      <h3>{connection ? 'Edit Connection' : 'New Connection'}</h3>
      <button class="modal-close" on:click={handleCancel} aria-label="Close dialog" title="Close"><Icon name="close" size={16} /></button>
    </div>

    <div class="modal-body">
      {#if errorMsg}
        <div
          style="background: var(--color-danger-light); color: var(--color-danger); padding: 8px 12px; border-radius: var(--radius-sm); margin-bottom: 16px; font-size: 13px;"
        >
          {errorMsg}
        </div>
      {/if}

      <div class="form-group">
        <label class="form-label" for="conn-name">Connection Name</label>
        <input
          id="conn-name"
          class="form-input"
          type="text"
          bind:value={name}
          placeholder="e.g. Production IIS"
        />
      </div>

      <div class="form-row">
        <div class="form-group">
          <label class="form-label" for="conn-host">Host / Server</label>
          <input
            id="conn-host"
            class="form-input"
            type="text"
            bind:value={host}
            placeholder="ftp.example.com or ftps://ftp.example.com/site/wwwroot"
          />
        </div>
        <div class="form-group">
          <label class="form-label" for="conn-port">Port</label>
          <input
            id="conn-port"
            class="form-input"
            type="number"
            bind:value={port}
            min="1"
            max="65535"
          />
        </div>
      </div>

      <div class="form-group">
        <label class="form-label" for="conn-protocol">Protocol</label>
        <select id="conn-protocol" class="form-select" bind:value={protocol}>
          <option value="ftps_explicit"
            >FTPS (port 21 explicit, port 990 implicit)</option
          >
          <option value="ftp">Standard FTP (Insecure)</option>
        </select>
      </div>

      <div class="form-row">
        <div class="form-group">
          <label class="form-label" for="conn-user">Username</label>
          <input
            id="conn-user"
            class="form-input"
            type="text"
            bind:value={username}
            autocomplete="off"
          />
        </div>
        <div class="form-group">
          <label class="form-label" for="conn-pass">
            Password {connection ? '(Leave blank to keep existing)' : ''}
          </label>
          <input
            id="conn-pass"
            class="form-input"
            type="password"
            bind:value={password}
            autocomplete="new-password"
          />
        </div>
      </div>

      <div class="form-group">
        <label class="form-label" for="conn-root">Remote Root Directory</label>
        <input
          id="conn-root"
          class="form-input"
          type="text"
          bind:value={remoteRoot}
          placeholder="/site/wwwroot or /"
        />
        <div class="form-hint">
          Target remote directory where build artifacts will be published. A
          path pasted in the server URL is moved here automatically.
        </div>
      </div>

      {#if protocol === 'ftps_explicit'}
        <div class="form-group">
          <label class="form-check">
            <input type="checkbox" bind:checked={tlsVerify} />
            <span
              >Verify TLS Certificate (Recommended: uncheck only for self-signed
              certificates)</span
            >
          </label>
        </div>
      {/if}

      <div class="form-actions">
        <button
          type="button"
          class="btn btn-secondary"
          on:click={handleTest}
          disabled={testing || saving}
        >
          {testing ? 'Testing...' : 'Test Connection'}
        </button>
        <button
          type="button"
          class="btn btn-ghost"
          on:click={handleCancel}
          disabled={saving}
        >
          Cancel
        </button>
        <button
          type="button"
          class="btn btn-primary"
          on:click={handleSave}
          disabled={saving || testing}
        >
          {saving ? 'Saving...' : 'Save Connection'}
        </button>
      </div>
    </div>
  </div>
</div>
