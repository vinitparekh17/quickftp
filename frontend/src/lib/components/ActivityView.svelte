<script lang="ts">
  import Icon from './Icon.svelte';
  import Pager from './Pager.svelte';
  import {
    groupTransfers,
    pageItems,
    isActiveTransfer,
    transferNeedsAttention,
  } from '../activityGroups';
  import { getDirectionIcon, getStatusIcon } from '../icons';
  import { formatBytes } from '../deploymentUtils';
  import {
    manualTransfers,
    connections,
    activityConnectionId,
    deployLogs,
    activityLogs,
    activityTab,
    currentPage,
    requestedDeploymentId,
    deployStatus,
    activeDeployment,
    addToast,
  } from '../stores';
  import {
    clearTransferHistory,
    clearDeploymentLogs,
    cancelTransfer,
    retryTransfer,
    removeTransfer,
    resolveTransferConflict,
  } from '../wailsjs';

  let scope = 'All';
  let search = '';
  let logCategory = 'All';
  let showProgress = false;
  let selectedGroupId: string | null = null;
  let page = 0;
  let groupPage = 0;
  let logPage = 0;
  const isActive = isActiveTransfer;
  const needsAttention = transferNeedsAttention;
  const priority: Record<string, number> = {
    Waiting: 0,
    Running: 1,
    Queued: 2,
    Failed: 3,
    Interrupted: 4,
  };
  $: groups =
    $activityTab === 'Transfers' ? groupTransfers($manualTransfers) : [];
  $: selectedGroup =
    groups.find((group) => group.id === selectedGroupId) || null;
  $: query = search.toLowerCase().trim();
  $: connectionGroups = [
    ...new Map([
      ...$connections.map(
        (connection) =>
          [connection.id, { id: connection.id, name: connection.name }] as const
      ),
      ...groups.map(
        (group) =>
          [
            group.connectionId,
            { id: group.connectionId, name: group.connectionName },
          ] as const
      ),
    ]).values(),
  ];
  $: connectionName =
    connectionGroups.find(
      (connection) => connection.id === $activityConnectionId
    )?.name || 'Connection';
  $: scopedGroups = groups.filter(
    (group) => group.connectionId === $activityConnectionId
  );
  $: matchingGroups = scopedGroups.filter((group) => {
    const filterScope = scope;
    const filterQuery = query;
    return group.entries.some((item) =>
      matches(item, filterScope, filterQuery)
    );
  });
  function matches(
    item: import('../types').TransferDTO,
    scope: string,
    query: string
  ) {
    if (scope === 'In progress' && !isActive(item.status)) return false;
    if (scope === 'Needs attention' && !needsAttention(item.status))
      return false;
    if (scope === 'Finished' && isActive(item.status)) return false;
    return `${item.remotePath} ${item.localPath} ${item.connectionName} ${item.status}`
      .toLowerCase()
      .includes(query);
  }
  $: visibleTransfers = selectedGroup
    ? selectedGroup.entries
        .filter((item) => {
          // Keep filter dependencies visible to Svelte's reactive compiler.
          return (
            (scope === 'All' ||
              (scope === 'In progress'
                ? isActive(item.status)
                : scope === 'Needs attention'
                  ? needsAttention(item.status)
                  : !isActive(item.status))) &&
            `${item.remotePath} ${item.localPath} ${item.connectionName} ${item.status}`
              .toLowerCase()
              .includes(query)
          );
        })
        .slice()
        .sort((a, b) => (priority[a.status] ?? 5) - (priority[b.status] ?? 5))
    : [];
  $: page = Math.min(
    page,
    Math.max(0, Math.ceil(visibleTransfers.length / 50) - 1)
  );
  $: groupPage = Math.min(
    groupPage,
    Math.max(0, Math.ceil(matchingGroups.length / 20) - 1)
  );
  $: pageTransfers = pageItems(visibleTransfers, page);
  $: scopedTransfers = $manualTransfers.filter(
    (item) =>
      $activityConnectionId === null ||
      item.connectionId === $activityConnectionId
  );
  $: scopedActive = scopedTransfers.filter((item) =>
    isActive(item.status)
  ).length;
  $: scopedAttention = scopedTransfers.filter((item) =>
    needsAttention(item.status)
  ).length;
  $: finished = scopedTransfers.filter((item) => !isActive(item.status));
  function selectGroup(id: string) {
    selectedGroupId = id;
    page = 0;
  }
  $: visibleLogs = $activityLogs.filter(
    (item) =>
      (showProgress || item.level !== 'PROGRESS') &&
      (logCategory === 'All' ||
        item.category === logCategory ||
        (logCategory === 'Errors' && item.level === 'ERROR')) &&
      `${item.message} ${item.category} ${item.level}`
        .toLowerCase()
        .includes(search.toLowerCase().trim())
  );
  $: logPage = Math.min(
    logPage,
    Math.max(0, Math.ceil(visibleLogs.length / 50) - 1)
  );
  async function call(action: () => Promise<unknown>) {
    try {
      await action();
    } catch (error: any) {
      addToast('error', error.message || String(error));
    }
  }
  function viewDeployment() {
    $requestedDeploymentId = 0;
    $currentPage = 'history';
  }
  async function copyLogs() {
    await call(async () => {
      await navigator.clipboard.writeText(
        visibleLogs
          .map(
            (item) =>
              `${new Date(item.timestamp).toLocaleString()} [${item.category}/${item.level}] ${item.message}`
          )
          .join('\n')
      );
      addToast('success', 'Visible log entries copied');
    });
  }
  async function clearHistory() {
    await call(() => clearTransferHistory($activityConnectionId ?? 0));
    selectedGroupId = null;
  }
  async function clearAllLogs() {
    await call(async () => {
      await clearDeploymentLogs(0);
      deployLogs.set([]);
      activityLogs.set([]);
    });
  }
</script>

<div class="view-full-height activity-page">
  <section class="page-heading">
    <div>
      <h1>Activity</h1>
      <p class="page-subtitle">
        Manual uploads and downloads belong to a connection. Project deployments
        and their logs are available under Projects.
      </p>
    </div>
    <div class="setup-actions">
      <button class="btn btn-secondary" on:click={clearHistory}
        >Clear finished transfer history</button
      >
      <button
        class="btn btn-secondary"
        on:click={() => ($currentPage = 'files')}
      >
        <Icon name="folderTree" size={15} /> Browse files
      </button>
    </div>
  </section>

  {#if $deployStatus === 'running'}
    <div class="context-notice">
      <span
        ><Icon name="loader" size={15} spin /> Deploying
        <strong>{$activeDeployment?.projectName || 'project'}</strong></span
      >
      <button class="text-button" on:click={viewDeployment}
        >View live deployment <Icon name="forward" size={13} /></button
      >
    </div>
  {/if}

  {#if $activityTab === 'Transfers'}<nav
      class="hierarchy-breadcrumb"
      aria-label="Connection activity navigation"
    >
      <button
        class="text-button"
        on:click={() => ($currentPage = 'connections')}>Connections</button
      ><span>/</span><button
        class="text-button"
        on:click={() => {
          $activityConnectionId = null;
          selectedGroupId = null;
        }}>Transfer history</button
      >{#if $activityConnectionId !== null}<span>/</span><span
          >{connectionName}</span
        >{/if}
    </nav>{/if}
  <div class="card activity-workspace">
    <nav class="workspace-tabs" aria-label="Activity views">
      <button
        class:active={$activityTab === 'Transfers'}
        aria-pressed={$activityTab === 'Transfers'}
        on:click={() => {
          $activityTab = 'Transfers';
          search = '';
        }}
      >
        <Icon name="transfer" size={15} /> Manual transfers
        {#if scopedActive}<span class="tab-count">{scopedActive}</span>{/if}
      </button>
      <button
        class:active={$activityTab === 'Logs'}
        aria-pressed={$activityTab === 'Logs'}
        on:click={() => {
          $activityTab = 'Logs';
          search = '';
        }}
      >
        <Icon name="fileText" size={15} /> Application log
      </button>
    </nav>

    {#if $activityTab === 'Transfers'}
      {#if selectedGroup}<div class="transfer-group-heading">
          <button
            class="text-button"
            on:click={() => {
              selectedGroupId = null;
              page = 0;
            }}><Icon name="back" size={14} /> Transfer batches</button
          ><strong>{selectedGroup.label}</strong><span class="text-muted"
            >{selectedGroup.entries.length.toLocaleString()} files</span
          >
        </div>{/if}
      <div class="workspace-toolbar">
        <div class="filter-pills" aria-label="Filter transfers">
          {#each ['All', 'In progress', 'Needs attention', 'Finished'] as option}
            <button
              class:active={scope === option}
              aria-pressed={scope === option}
              on:click={() => {
                scope = option;
                page = 0;
                groupPage = 0;
              }}
            >
              {option}
              {#if option === 'In progress'}<span>{scopedActive}</span
                >{:else if option === 'Needs attention'}<span
                  >{scopedAttention}</span
                >{:else if option === 'Finished'}<span>{finished.length}</span
                >{/if}
            </button>
          {/each}
        </div>
        <input
          class="form-input workspace-search"
          aria-label="Search transfers"
          placeholder="Find a file or connection…"
          bind:value={search}
          on:input={() => {
            page = 0;
            groupPage = 0;
            logPage = 0;
          }}
        />
      </div>
      <div class="workspace-scroll">
        {#if $activityConnectionId === null}
          <div class="activity-groups">
            {#each connectionGroups as connection}<button
                class="activity-group-card"
                on:click={() => {
                  $activityConnectionId = connection.id;
                  selectedGroupId = null;
                  groupPage = 0;
                }}
                ><span class="activity-group-title"
                  ><Icon name="server" size={17} /><strong
                    >{connection.name}</strong
                  ></span
                ><span class="cell-secondary"
                  >{groups.filter(
                    (group) => group.connectionId === connection.id
                  ).length} transfer batches</span
                ><span class="activity-group-link"
                  >View this connection’s transfer history</span
                ></button
              >{:else}<div class="empty-state">
                No manual transfers yet. Start uploads or downloads in Files.
              </div>{/each}
          </div>
        {:else if !selectedGroup}
          <div class="activity-groups">
            {#each pageItems(matchingGroups, groupPage, 20) as group (group.id)}
              <button
                class="activity-group-card"
                on:click={() => selectGroup(group.id)}
              >
                <span class="activity-group-title"
                  ><Icon name="folder" size={17} /><strong>{group.label}</strong
                  >{#if group.recovered}<span class="badge badge-gray"
                      >Recovered / unassigned</span
                    >{/if}</span
                >
                <span class="cell-secondary"
                  >{group.connectionName} · {group.entries.length.toLocaleString()}
                  files · {group.active.toLocaleString()} in progress · {group.attention.toLocaleString()}
                  need attention</span
                >
                {#if group.recovered}<span class="cell-secondary"
                    >Older records have no deployment or batch ID. Grouped by
                    connection and start date; all original file details are
                    retained.</span
                  >{/if}
                <span class="activity-group-link"
                  >Inspect files &amp; errors <Icon
                    name="forward"
                    size={13}
                  /></span
                >
              </button>
            {:else}<div class="empty-state">
                <h3>No matching transfer batches</h3>
                <p>Manual transfers started in Files appear here.</p>
              </div>{/each}
          </div>
        {:else if visibleTransfers.length}
          <table class="transfer-table">
            <thead
              ><tr
                ><th>File &amp; connection</th><th>Status</th><th>Progress</th
                ><th class="row-actions-heading">Actions</th></tr
              ></thead
            >
            <tbody>
              {#each pageTransfers as item (item.id)}
                <tr>
                  <td class="transfer-file-cell">
                    <details class="transfer-file-details">
                      <summary
                        ><Icon
                          name={getDirectionIcon(item.direction)}
                          size={15}
                        /><strong>{item.remotePath.split('/').pop()}</strong
                        ></summary
                      >
                      <dl>
                        <dt>Remote</dt>
                        <dd>{item.remotePath}</dd>
                        <dt>Local</dt>
                        <dd>{item.localPath}</dd>
                        {#if item.verification}<dt>Verified by</dt>
                          <dd>{item.verification}</dd>{/if}
                      </dl>
                    </details>
                    <span class="cell-secondary"
                      >{item.connectionName} · {item.direction}</span
                    >
                    {#if item.error}<p class="transfer-problem">
                        {item.error}
                      </p>{/if}
                    {#if item.status === 'Waiting'}
                      <div class="conflict-actions">
                        <span>Destination exists:</span>
                        {#each ['Skip', 'Rename', 'Overwrite'] as policy}<button
                            class="btn btn-secondary btn-sm"
                            on:click={() =>
                              call(() =>
                                resolveTransferConflict(item.id, policy)
                              )}>{policy}</button
                          >{/each}
                      </div>
                    {/if}
                  </td>
                  <td
                    ><span
                      class="transfer-status"
                      class:is-error={['Failed', 'Interrupted'].includes(
                        item.status
                      )}
                      class:is-success={item.status === 'Completed'}
                      ><Icon
                        name={getStatusIcon(item.status)}
                        size={13}
                        spin={item.status === 'Running'}
                      />{item.status}</span
                    ></td
                  >
                  <td class="transfer-progress-cell">
                    <span
                      >{formatBytes(item.bytes)}{item.total > 0
                        ? ` / ${formatBytes(item.total)}`
                        : ''}</span
                    >
                    {#if isActive(item.status) && item.total > 0}<progress
                        aria-label={`Progress for ${item.remotePath}`}
                        max={item.total}
                        value={item.bytes}
                      ></progress>{/if}
                  </td>
                  <td class="row-actions-heading">
                    {#if isActive(item.status)}<button
                        class="text-button"
                        on:click={() => call(() => cancelTransfer(item.id))}
                        >Cancel</button
                      >
                    {:else}
                      <div class="table-actions">
                        {#if ['Failed', 'Interrupted', 'Cancelled'].includes(item.status)}<button
                            class="btn btn-secondary btn-sm"
                            on:click={() => call(() => retryTransfer(item.id))}
                            ><Icon name="refresh" size={13} /> Retry</button
                          >{/if}
                        <button
                          class="table-icon-action action-secondary"
                          aria-label={`Dismiss ${item.remotePath}`}
                          title="Dismiss transfer"
                          on:click={() => call(() => removeTransfer(item.id))}
                          ><Icon name="close" size={15} /></button
                        >
                      </div>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        {:else}
          <div class="empty-state">
            <Icon name="transfer" size={30} />
            <h3>
              {search || scope !== 'All'
                ? 'No matching transfers'
                : 'No file transfers yet'}
            </h3>
            <p>
              {search || scope !== 'All'
                ? 'Try another filter or search.'
                : 'Uploads and downloads started in Files appear here. Transfers continue when you leave this page.'}
            </p>
            {#if !search && scope === 'All'}<button
                class="btn btn-secondary"
                on:click={() => ($currentPage = 'files')}>Browse files</button
              >{/if}
          </div>
        {/if}
      </div>
      {#if selectedGroup}<Pager
          bind:page
          total={visibleTransfers.length}
        />{:else}<Pager
          bind:page={groupPage}
          total={matchingGroups.length}
          size={20}
        />{/if}
    {:else}
      <div class="workspace-toolbar">
        <select
          class="form-select log-category"
          aria-label="Filter log category"
          bind:value={logCategory}
          >{#each ['All', 'Errors', 'Build', 'Deployment', 'Backup', 'Transfer'] as category}<option
              >{category}</option
            >{/each}</select
        >
        <input
          class="form-input workspace-search"
          aria-label="Search application logs"
          placeholder="Search logs…"
          bind:value={search}
          on:input={() => {
            page = 0;
            groupPage = 0;
            logPage = 0;
          }}
        />
        <label class="form-check log-progress-toggle"
          ><input type="checkbox" bind:checked={showProgress} /> Show progress entries</label
        >
        <button
          class="btn btn-secondary btn-sm"
          on:click={copyLogs}
          disabled={!visibleLogs.length}
          ><Icon name="copy" size={14} /> Copy filtered</button
        >
      </div>
      <div class="workspace-toolbar">
        <button class="text-button" on:click={() => activityLogs.set([])}
          >Clear application log</button
        ><button
          class="text-button"
          title="Clears text logs only; deployment records, file journals and backups remain."
          on:click={clearAllLogs}>Clear all text logs</button
        >
      </div>
      <div
        class="application-log"
        role="region"
        aria-label="Application log, selectable text"
      >
        {#each pageItems(visibleLogs, logPage) as item}
          <div class="application-log-row">
            <time>{new Date(item.timestamp).toLocaleTimeString()}</time><span
              class="log-level"
              class:is-error={item.level === 'ERROR'}
              >{item.category} / {item.level}</span
            ><span class="log-message">{item.message}</span>
          </div>
        {:else}<div class="log-empty">
            {search || logCategory !== 'All'
              ? 'No log entries match your filters.'
              : 'Application activity will appear here. You can select text or copy the visible log.'}
          </div>{/each}
      </div>
      <Pager bind:page={logPage} total={visibleLogs.length} />
    {/if}
  </div>
</div>
