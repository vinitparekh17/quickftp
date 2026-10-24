<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import Icon from '../Icon.svelte';
  import { getProjectIcon } from '../../icons';
  import type { ProjectDTO, ConnectionDTO } from '../../types';
  import { deployStatus, addToast } from '../../stores';
  import { startDeploy } from '../../wailsjs';

  export let projects: ProjectDTO[] = [];
  export let connections: ConnectionDTO[] = [];
  export let initialProjectName = '';
  export let initialConnectionName = '';

  const dispatch = createEventDispatcher<{
    close: void;
    deployed: { projectName: string; connectionName: string };
  }>();

  let selectedProject =
    initialProjectName || (projects.length > 0 ? projects[0].name : '');
  let selectedConnection =
    initialConnectionName || (connections.length > 0 ? connections[0].name : '');

  let enableIISOffline = false;
  let iisShutdownDelay = 1500;
  let iisOfflineHtml =
    '<!DOCTYPE html><html><head><title>App Offline</title></head><body><h1>Maintenance in Progress</h1><p>The application is currently being updated. Please check back shortly.</p></body></html>';

  let isSubmitting = false;

  $: activeProject = projects.find((p) => p.name === selectedProject) || null;
  $: activeConnection =
    connections.find((c) => c.name === selectedConnection) || null;

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && !isSubmitting) {
      dispatch('close');
    }
  }

  async function handleStart() {
    if (!selectedProject) {
      addToast('error', 'Please select a project to deploy');
      return;
    }
    if (!selectedConnection) {
      addToast('error', 'Please select a target FTP connection');
      return;
    }
    if ($deployStatus === 'running' || isSubmitting) {
      return;
    }

    isSubmitting = true;
    try {
      await startDeploy(selectedProject, selectedConnection, {
        iisOfflinePage: enableIISOffline ? iisOfflineHtml : '',
        iisShutdownDelay: Number(iisShutdownDelay) || 1500,
      });

      addToast(
        'info',
        `Started new deployment for ${selectedProject} → ${selectedConnection}`
      );
      dispatch('deployed', {
        projectName: selectedProject,
        connectionName: selectedConnection,
      });
      dispatch('close');
    } catch (err: any) {
      addToast('error', `Could not start deployment: ${err.message}`);
    } finally {
      isSubmitting = false;
    }
  }
</script>

<svelte:window on:keydown={handleKeydown} />

<!-- Accessible modal backdrop -->
<div
  class="modal-backdrop"
  on:click={() => !isSubmitting && dispatch('close')}
  role="presentation"
>
  <!-- svelte-ignore a11y-click-events-have-key-events -->
  <!-- svelte-ignore a11y-no-noninteractive-element-interactions -->
  <div
    class="modal-box new-deploy-modal"
    on:click|stopPropagation
    role="dialog"
    aria-modal="true"
    aria-labelledby="modal-deploy-title"
  >
    <div class="modal-header">
      <div class="modal-title-wrap">
        <Icon name="deploy" size={18} />
        <h3 id="modal-deploy-title">New Deployment</h3>
      </div>
      <button
        class="table-icon-action action-secondary"
        on:click={() => dispatch('close')}
        disabled={isSubmitting}
        title="Close dialog"
        aria-label="Close dialog"
      >
        <Icon name="close" size={16} />
      </button>
    </div>

    <div class="modal-body new-deploy-body">
      <!-- Section 1: Target Selections -->
      <div class="deploy-flow-step">
        <div class="step-label">1. Select Project &amp; Target Host</div>
        <div class="form-row">
          <div class="form-group mb-2">
            <label class="form-label" for="new-deploy-proj">Project</label>
            <select
              id="new-deploy-proj"
              class="form-select"
              bind:value={selectedProject}
              disabled={isSubmitting || projects.length === 0}
            >
              {#if projects.length === 0}
                <option value="">No projects saved</option>
              {:else}
                {#each projects as p}
                  <option value={p.name}>
                    {p.name} ({p.projectType === 'angular' ? 'Angular' : '.NET'})
                  </option>
                {/each}
              {/if}
            </select>
          </div>

          <div class="form-group mb-2">
            <label class="form-label" for="new-deploy-conn">Target Connection</label>
            <select
              id="new-deploy-conn"
              class="form-select"
              bind:value={selectedConnection}
              disabled={isSubmitting || connections.length === 0}
            >
              {#if connections.length === 0}
                <option value="">No connections saved</option>
              {:else}
                {#each connections as c}
                  <option value={c.name}>
                    {c.name} ({c.protocol === 'ftps_explicit' ? 'FTPS' : 'FTP'} • {c.host})
                  </option>
                {/each}
              {/if}
            </select>
          </div>
        </div>
      </div>

      <!-- Section 2: Configuration Review -->
      {#if activeProject && activeConnection}
        <div class="deploy-flow-step">
          <div class="step-label">2. Review Configuration</div>
          <div class="config-review-card">
            <div class="config-item">
              <span class="config-k">Build Command</span>
              <span class="config-v text-mono">{activeProject.buildCommand || '(no build command)'}</span>
            </div>
            <div class="config-item">
              <span class="config-k">Source Artifacts</span>
              <span class="config-v text-mono">{activeProject.outputRelPath || '.'}</span>
            </div>
            <div class="config-item">
              <span class="config-k">Remote Root</span>
              <span class="config-v text-mono">{activeConnection.remoteRoot || '/'}</span>
            </div>
            <div class="config-item">
              <span class="config-k">Security</span>
              <span class="config-v">
                {activeConnection.protocol === 'ftps_explicit' ? 'FTPS Explicit (TLS 1.2+)' : 'Standard FTP'}
                {#if activeConnection.tlsVerify}
                  <span class="text-success text-xs font-semibold ml-1">✓ Verified</span>
                {/if}
              </span>
            </div>
          </div>
        </div>
      {/if}

      <!-- Section 3: IIS Maintenance Mode Option (Optional) -->
      <div class="deploy-flow-step">
        <div class="step-label">3. Optional IIS Quiescence</div>
        <div class="offline-option-toggle">
          <label class="form-check">
            <input
              type="checkbox"
              bind:checked={enableIISOffline}
              disabled={isSubmitting}
            />
            <span class="text-sm">
              Enable IIS <code>app_offline.htm</code> quiescence during file swap
            </span>
          </label>
        </div>

        {#if enableIISOffline}
          <div class="offline-config-card mt-2">
            <div class="form-group mb-2">
              <label class="form-label" for="new-deploy-delay">
                Quiescence Shutdown Delay (milliseconds)
              </label>
              <input
                id="new-deploy-delay"
                class="form-input"
                type="number"
                min="500"
                step="500"
                style="max-width: 160px;"
                bind:value={iisShutdownDelay}
                disabled={isSubmitting}
              />
              <p class="form-hint">Allows ASP.NET Core process time to release locked DLL handles.</p>
            </div>
            <div class="form-group mb-0">
              <label class="form-label" for="new-deploy-banner">Maintenance HTML Banner</label>
              <textarea
                id="new-deploy-banner"
                class="form-textarea text-xs"
                rows="2"
                bind:value={iisOfflineHtml}
                disabled={isSubmitting}
              ></textarea>
            </div>
          </div>
        {/if}
      </div>

      <!-- Section 4: Safety Plan & Destructive Consequences -->
      <div class="deploy-flow-step">
        <div class="step-label">4. Execution Plan &amp; Safety Guarantees</div>
        <div class="safety-notice-card">
          <ul class="safety-steps-list">
            <li>
              <span class="step-bullet">1</span>
              <span><strong>Pre-flight Backup:</strong> Current remote files are downloaded to an isolated local snapshot directory before remote modification.</span>
            </li>
            <li>
              <span class="step-bullet">2</span>
              <span><strong>Staging Upload:</strong> New artifacts are uploaded with unique temporary filenames (<code>.quickftp-*.tmp</code>).</span>
            </li>
            <li>
              <span class="step-bullet">3</span>
              <span><strong>Digest Verification:</strong> SHA-256 digests are verified remotely before atomic promotion.</span>
            </li>
            <li>
              <span class="step-bullet">4</span>
              <span><strong>Controlled Pruning:</strong> Stale remote files not in the new build are removed only after their verified local backup is safely stored.</span>
            </li>
          </ul>
        </div>
      </div>
    </div>

    <div class="modal-footer">
      <button
        class="btn btn-secondary"
        on:click={() => dispatch('close')}
        disabled={isSubmitting}
      >
        Cancel
      </button>

      <button
        class="btn btn-primary"
        on:click={handleStart}
        disabled={isSubmitting || !selectedProject || !selectedConnection || $deployStatus === 'running'}
      >
        {#if isSubmitting}
          <Icon name="loader" size={14} spin={true} />
          <span>Starting deployment...</span>
        {:else}
          <Icon name="play" size={14} />
          <span>Confirm &amp; Start Deployment</span>
        {/if}
      </button>
    </div>
  </div>
</div>

<style>
  .new-deploy-modal {
    max-width: 540px;
    width: 95%;
  }

  .modal-title-wrap {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 600;
  }

  .modal-title-wrap h3 {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
  }

  .new-deploy-body {
    padding: 16px 20px;
    max-height: 70vh;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .deploy-flow-step {
    display: flex;
    flex-direction: column;
  }

  .step-label {
    font-size: 11px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-text-secondary);
    margin-bottom: 8px;
  }

  .config-review-card {
    background: var(--color-input-bg);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    padding: 10px 14px;
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px 16px;
  }

  .config-item {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .config-k {
    font-size: 10px;
    font-weight: 600;
    color: var(--color-text-light);
    text-transform: uppercase;
  }

  .config-v {
    font-size: 12px;
    font-weight: 500;
    color: var(--color-text);
    word-break: break-all;
  }

  .offline-option-toggle {
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    padding: 8px 12px;
  }

  .offline-config-card {
    background: var(--color-input-bg);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    padding: 12px;
  }

  .safety-notice-card {
    background: #f8fafc;
    border: 1px solid #e2e8f0;
    border-radius: var(--radius-sm);
    padding: 10px 14px;
  }

  .safety-steps-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
    font-size: 12px;
    line-height: 1.45;
    color: var(--color-text);
  }

  .safety-steps-list li {
    display: flex;
    align-items: flex-start;
    gap: 8px;
  }

  .step-bullet {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    background: #e2e8f0;
    color: #475569;
    font-size: 10px;
    font-weight: 700;
    flex-shrink: 0;
    margin-top: 1px;
  }
</style>

