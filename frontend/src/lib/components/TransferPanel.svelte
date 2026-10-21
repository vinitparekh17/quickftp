<script lang="ts">
  import { onMount } from 'svelte';
  import {
    transfers,
    activityLogs,
    activityPanelOpen,
    activityPanelTab,
    activityPanelHeight,
    acknowledgedFailures,
    addToast,
  } from '../stores';
  import Icon from './Icon.svelte';
  import { getDirectionIcon, getStatusIcon } from '../icons';
  import {
    getTransfers,
    cancelTransfer,
    retryTransfer,
    removeTransfer,
    resolveTransferConflict,
  } from '../wailsjs';
  let filter = 'All';
  let logBox: HTMLDivElement;
  let resizing = false;
  $: safeTransfers = Array.isArray($transfers) ? $transfers : [];
  $: safeLogs = Array.isArray($activityLogs) ? $activityLogs : [];
  $: active = safeTransfers.filter((t) =>
    ['Queued', 'Running', 'Waiting'].includes(t.status)
  );
  $: failures = safeTransfers.filter(
    (t) => t.status === 'Failed' || t.status === 'Interrupted'
  );
  $: unacknowledged = failures.filter(
    (t) => !$acknowledgedFailures.includes(t.id)
  );
  $: history = safeTransfers.filter(
    (t) => !['Queued', 'Running', 'Waiting'].includes(t.status)
  );
  $: visibleLogs = safeLogs.filter(
    (e) =>
      filter === 'All' ||
      e.category === filter ||
      (filter === 'Errors' && e.level === 'ERROR') ||
      (filter === 'Success' && e.level === 'SUCCESS')
  );
  onMount(() => {
    (async () => {
      try {
        $transfers = (await getTransfers()) || [];
      } catch (e: any) {
        addToast('error', `Could not load transfer activity: ${e.message}`);
      }
    })();

    const move = (e: MouseEvent) => {
      if (!resizing) return;
      const h = Math.max(180, Math.min(520, window.innerHeight - e.clientY));
      $activityPanelHeight = h;
      localStorage.setItem('quickftp.activity.height', String(h));
    };
    const up = () => (resizing = false);
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', up);
    return () => {
      window.removeEventListener('mousemove', move);
      window.removeEventListener('mouseup', up);
    };
  });
  function toggle() {
    $activityPanelOpen = !$activityPanelOpen;
    localStorage.setItem('quickftp.activity.open', String($activityPanelOpen));
  }
  async function call(fn: () => Promise<any>) {
    try {
      await fn();
    } catch (e: any) {
      addToast('error', e.message);
    }
  }
  function acknowledge() {
    const ids = failures.map((t) => t.id);
    $acknowledgedFailures = [...new Set([...$acknowledgedFailures, ...ids])];
    localStorage.setItem(
      'quickftp.activity.acknowledged',
      JSON.stringify($acknowledgedFailures)
    );
  }
  async function copy() {
    await navigator.clipboard.writeText(
      visibleLogs
        .map(
          (x) =>
            `${new Date(x.timestamp).toLocaleString()} [${x.category}/${x.level}] ${x.message}`
        )
        .join('\n')
    );
    addToast('success', 'Activity log copied');
  }
  function formatSize(n: number) {
    if (n < 1024) return `${n} B`;
    const u = ['KB', 'MB', 'GB', 'TB'];
    let i = -1;
    do {
      n /= 1024;
      i++;
    } while (n >= 1024 && i < u.length - 1);
    return `${n.toFixed(1)} ${u[i]}`;
  }
</script>

<section class="activity-dock" class:expanded={$activityPanelOpen}>
  <!-- svelte-ignore a11y-no-noninteractive-element-interactions -->
  {#if $activityPanelOpen}<div
  role="separator"
      class="activity-resize"
      on:mousedown={() => (resizing = true)}
      aria-label="Resize activity panel"
    ></div>{/if}
  <header class="activity-bar">
    <button
      class="activity-toggle"
      title={$activityPanelOpen ? 'Collapse activity' : 'Expand activity'}
      on:click={toggle}
      aria-expanded={$activityPanelOpen}
      ><span class="activity-chevron"><Icon name={$activityPanelOpen ? 'chevronDown' : 'chevronUp'} size={14} /></span
      ><strong>Activity</strong><span class="activity-count"
        >{active.length} running · {active.filter((x) => x.status === 'Queued')
          .length} queued</span
      >{#if unacknowledged.length}<span class="activity-failure"
          >{unacknowledged.length} failed</span
        >{:else if failures.length}<span class="activity-ack"
          >{failures.length} failures reviewed</span
        >{/if}</button
    ><button
      class="activity-open-tab"
      on:click={() => {
        $activityPanelTab = 'Transfers';
        if (!$activityPanelOpen) toggle();
      }}
      >{#if active.length}{active.length} transfer{active.length === 1
          ? ''
          : 's'} running{:else}Transfer activity{/if}</button
    >
  </header>
  {#if $activityPanelOpen}<div
      class="activity-panel-body"
      style={`height:${$activityPanelHeight}px`}
    >
      <nav class="activity-tabs">
        <button
          class:active={$activityPanelTab === 'Transfers'}
          on:click={() => ($activityPanelTab = 'Transfers')}
          >Transfers <span>{active.length}</span></button
        ><button
          class:active={$activityPanelTab === 'Logs'}
          on:click={() => ($activityPanelTab = 'Logs')}>Logs</button
        ><span class="activity-tab-spacer"
        ></span>{#if unacknowledged.length}<button
            class="text-button"
            on:click={acknowledge}>Acknowledge failures</button
          >{/if}
      </nav>
      {#if $activityPanelTab === 'Transfers'}<div
          class="activity-transfer-list"
        >
          {#each active as t (t.id)}<article class="transfer-row">
              <div class="transfer-row-title">
                <span class="transfer-dir-icon">
                  <Icon name={getDirectionIcon(t.direction)} size={14} />
                </span>
                <strong>{t.remotePath.split('/').pop()}</strong><span
                  class="badge"
                  class:badge-warning={t.status === 'Waiting'}
                  ><Icon name={getStatusIcon(t.status)} size={12} /> {t.direction} · {t.status}</span
                >
              </div>
              <small>{t.connectionName}: {t.remotePath} → {t.localPath}</small
              >{#if t.total > 0}<progress max={t.total} value={t.bytes}
                ></progress><small
                  >{formatSize(t.bytes)} / {formatSize(t.total)}</small
                >{/if}{#if t.error}<small class="transfer-error"
                  >{t.error}</small
                >{/if}{#if t.status === 'Waiting'}<div class="conflict-actions">
                  <span>Destination exists:</span
                  >{#each ['Skip', 'Rename', 'Overwrite'] as p}<button
                      class="btn btn-secondary btn-sm"
                      on:click={() =>
                        call(() => resolveTransferConflict(t.id, p))}
                      >{p}</button
                    >{/each}
                </div>{/if}
              <div class="transfer-actions">
                <button
                  class="text-button"
                  on:click={() => call(() => cancelTransfer(t.id))}
                  ><Icon name="close" size={13} /> Cancel</button
                >
              </div>
            </article>{:else}<div class="transfer-empty">
              <Icon name="transfer" size={24} /> No active transfers.
            </div>{/each}
          {#if history.length}<h3 class="activity-history-title">
              Transfer history
            </h3>
            {#each history as t (t.id)}<article class="transfer-row">
                <div class="transfer-row-title">
                  <span class="transfer-dir-icon">
                    <Icon name={getDirectionIcon(t.direction)} size={14} />
                  </span>
                  <strong>{t.remotePath.split('/').pop()}</strong><span
                    class="badge"><Icon name={getStatusIcon(t.status)} size={12} /> {t.direction} · {t.status}</span
                  >
                </div>
                <small>{t.connectionName}: {t.remotePath} → {t.localPath}</small
                >{#if t.total > 0}<small
                    >{formatSize(t.bytes)} / {formatSize(t.total)}</small
                  >{/if}{#if t.error}<small class="transfer-error"
                    >{t.error}</small
                  >{/if}{#if t.verification}<small
                    >Verification: {t.verification}</small
                  >{/if}
                <div class="transfer-actions">
                  {#if ['Failed', 'Interrupted', 'Cancelled'].includes(t.status) && t.direction !== 'Deployment upload'}<button
                      class="text-button"
                      on:click={() => call(() => retryTransfer(t.id))}
                      ><Icon name="refresh" size={13} /> Retry</button
                    >{/if}<button
                    class="text-button"
                    on:click={() => call(() => removeTransfer(t.id))}
                    ><Icon name="trash" size={13} /> Remove</button
                  >
                </div>
              </article>{/each}{/if}
        </div>
      {:else}<div class="activity-logs">
          <div class="transfer-log-heading">
            <strong>Application activity</strong><select
              class="form-select"
              bind:value={filter}
              ><option>All</option><option>Build</option><option
                >Deployment</option
              ><option>Backup</option><option>Transfer</option><option
                >Errors</option
              ><option>Success</option></select
            ><button class="text-button" on:click={copy}><Icon name="copy" size={13} /> Copy</button>
          </div>
          <div class="transfer-log-items" bind:this={logBox}>
            {#each visibleLogs as e}<div>
                <time>{new Date(e.timestamp).toLocaleString()}</time><b
                  class:transfer-error={e.level === 'ERROR'}
                  >{e.category}/{e.level}</b
                ><span>{e.message}</span>
              </div>{/each}
          </div>
        </div>{/if}
    </div>{/if}
</section>
