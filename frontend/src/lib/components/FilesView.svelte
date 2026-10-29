<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import {
    connections,
    remotePathByConnection,
    currentPage,
    activityTab,
    activityConnectionId,
    transferSummary,
    addToast,
  } from '../stores';
  import {
    getConnections,
    listRemoteDirectory,
    queueDownload,
    queueUpload,
    queueUploadDirectory,
    queueDownloadDirectory,
    selectDirectory,
    selectFiles,
    getSettings,
    deleteRemoteEntries,
  } from '../wailsjs';
  import type { ConnectionDTO, RemoteFileDTO } from '../types';
  import { pointerSelection, keyboardSelection } from '../selection';
  import Icon from './Icon.svelte';
  import { getFileIcon } from '../icons';
  let connectionId = '';
  let entries: RemoteFileDTO[] = [];
  let loading = false;
  let error = '';
  let deleting = false;
  let query = '';
  let selected: string[] = [];
  let anchor = -1;
  let focusIndex = -1;
  let downloadDir = '';
  let rows: HTMLElement;
  let pathInput = '';
  let back: string[] = [];
  let forward: string[] = [];
  let loadGeneration = 0;
  const pendingDirectoryLoads = new Map<string, Promise<RemoteFileDTO[]>>();
  onDestroy(() => {
    loadGeneration++;
  });
  $: connection = $connections.find((c) => String(c.id) === connectionId);
  $: currentPath = connection
    ? $remotePathByConnection[connection.id] || connection.remoteRoot || '/'
    : '/';
  $: filtered = (entries || [])
    .filter((e) =>
      e.path.split('/').pop()?.toLowerCase().includes(query.toLowerCase())
    )
    .sort(
      (a, b) =>
        Number(b.isDir) - Number(a.isDir) || a.path.localeCompare(b.path)
    );
  onMount(async () => {
    try {
      $connections = await getConnections();
      const saved = localStorage.getItem('quickftp.files.connection');
      connectionId =
        saved && $connections.some((c) => String(c.id) === saved)
          ? saved
          : $connections[0]
            ? String($connections[0].id)
            : '';
      downloadDir = (await getSettings()).downloadDir;
    } catch (e: any) {
      error = e.message;
    }
  });
  $: if (connectionId && connection) {
    localStorage.setItem('quickftp.files.connection', connectionId);
    if (!$remotePathByConnection[connection.id])
      $remotePathByConnection = {
        ...$remotePathByConnection,
        [connection.id]: connection.remoteRoot || '/',
      };
  }
  $: if (connectionId && currentPath) {
    pathInput = currentPath;
    loadDirectory(connectionId, currentPath);
  }
  let lastLoad = '';
  function changeConnection() {
    back = [];
    forward = [];
    selected = [];
    anchor = -1;
    focusIndex = -1;
    lastLoad = '';
  }
  async function loadDirectory(id: string, p: string, force = false) {
    const requestedPath = p.replace(/\/+$/, '') || '/';
    const key = id + '|' + requestedPath;
    if ((!force && lastLoad === key) || !$connections.length) return;
    lastLoad = key;
    const generation = ++loadGeneration;
    loading = true;
    error = '';
    selected = [];
    try {
      // Coalesce rapid back/forward clicks before they become FTP requests.
      // A refresh bypasses this delay and starts a fresh listing immediately.
      if (!force)
        await new Promise<void>((resolve) => setTimeout(resolve, 120));
      if (generation !== loadGeneration) return;
      let request = force ? undefined : pendingDirectoryLoads.get(key);
      if (!request) {
        let pending!: Promise<RemoteFileDTO[]>;
        pending = listRemoteDirectory(Number(id), requestedPath).finally(() => {
          if (pendingDirectoryLoads.get(key) === pending) {
            pendingDirectoryLoads.delete(key);
          }
        });
        pendingDirectoryLoads.set(key, pending);
        request = pending;
      }
      const result = await request;
      if (generation === loadGeneration)
        entries = Array.isArray(result) ? result : [];
    } catch (e: any) {
      if (generation === loadGeneration) {
        error = e?.message || 'Unable to list this directory';
        entries = [];
      }
    } finally {
      if (generation === loadGeneration) loading = false;
    }
  }
  function setPath(p: string, push = true) {
    if (!connection) return;
    if (push && p !== currentPath) {
      back = [...back, currentPath];
      forward = [];
    }
    $remotePathByConnection = {
      ...$remotePathByConnection,
      [connection.id]: p,
    };
    lastLoad = '';
  }
  function goBack() {
    const p = back.at(-1);
    if (p) {
      back = back.slice(0, -1);
      forward = [currentPath, ...forward];
      setPath(p, false);
    }
  }
  function goForward() {
    const p = forward[0];
    if (p) {
      forward = forward.slice(1);
      back = [...back, currentPath];
      setPath(p, false);
    }
  }
  function up() {
    if (!connection) return;
    const root = (connection.remoteRoot || '/').replace(/\/$/, '') || '/';
    if (currentPath === root) return;
    let p =
      currentPath.replace(/\/$/, '').split('/').slice(0, -1).join('/') || '/';
    if (root !== '/' && p.length < root.length) p = root;
    setPath(p);
  }
  function open(e: RemoteFileDTO) {
    if (e.isDir) setPath(e.path);
  }
  function choose(e: RemoteFileDTO, event: MouseEvent) {
    const i = filtered.findIndex((x) => x.path === e.path);
    const result = pointerSelection(filtered, selected, anchor, i, event);
    selected = result.selected;
    anchor = result.anchor;
    focusIndex = i;
  }
  function keydown(event: KeyboardEvent) {
    if (!filtered.length) return;
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      focusIndex = Math.max(
        0,
        Math.min(
          filtered.length - 1,
          focusIndex + (event.key === 'ArrowDown' ? 1 : -1)
        )
      );
      const result = keyboardSelection(
        filtered,
        selected,
        anchor,
        focusIndex,
        event.shiftKey
      );
      if (!event.ctrlKey && !event.metaKey) {
        selected = result.selected;
        anchor = result.anchor;
      }
    } else if (
      (event.ctrlKey || event.metaKey) &&
      event.key.toLowerCase() === 'a'
    ) {
      event.preventDefault();
      selected = filtered.map((x) => x.path);
    } else if (event.key === 'Delete' && selected.length > 0) {
      event.preventDefault();
      deleteSelected();
    } else if (event.key === 'Enter' && focusIndex >= 0) {
      open(filtered[focusIndex]);
    } else if (event.key === 'Escape') {
      selected = [];
    }
  }
  function localDestination(remote: string) {
    if (!connection) return '';
    const root = (connection.remoteRoot || '/').replace(/\/$/, '');
    const rel = remote.startsWith(root + '/')
      ? remote.slice(root.length + 1)
      : remote.split('/').pop() || 'file';
    return `${downloadDir.replace(/[\\/]$/, '')}\\${rel.replaceAll('/', '\\')}`;
  }
  async function download(paths = selected) {
    let count = 0;
    for (const p of paths) {
      const f = entries.find((x) => x.path === p);
      if (!f) continue;
      try {
        if (f.isDir) {
          const jobs = await queueDownloadDirectory(Number(connectionId), p);
          count += jobs.length;
        } else {
          await queueDownload(
            Number(connectionId),
            p,
            localDestination(p),
            f.sizeKnown ? f.size : 0
          );
          count++;
        }
      } catch (e: any) {
        addToast('error', `Could not queue ${p}: ${e.message}`);
      }
    }
    if (count > 0) addToast('info', `${count} file(s) queued for download`);
  }
  async function deleteSelected() {
    if (!connection || !selected.length || deleting) return;
    const itemsToDelete: { path: string; isDir: boolean }[] = [];
    for (const p of selected) {
      const found = entries.find((x) => x.path === p);
      if (found) {
        itemsToDelete.push({ path: found.path, isDir: found.isDir });
      }
    }
    if (!itemsToDelete.length) return;

    const hasFolder = itemsToDelete.some((x) => x.isDir);
    let msg = '';
    if (itemsToDelete.length === 1) {
      const item = itemsToDelete[0];
      const name = item.path.split('/').pop() || item.path;
      msg = item.isDir
        ? `Are you sure you want to permanently delete folder "${name}" and all of its contents?`
        : `Are you sure you want to permanently delete "${name}"?`;
    } else {
      msg = hasFolder
        ? `Are you sure you want to permanently delete ${itemsToDelete.length} selected items (including all folder contents)?`
        : `Are you sure you want to permanently delete ${itemsToDelete.length} selected files?`;
    }

    if (!confirm(msg)) return;

    deleting = true;
    try {
      await deleteRemoteEntries(connection.id, itemsToDelete);
      addToast(
        'success',
        `Successfully deleted ${itemsToDelete.length} item(s)`
      );
      selected = [];
      refresh();
    } catch (err: any) {
      addToast('error', `Failed to delete: ${err?.message || err}`);
      refresh();
    } finally {
      deleting = false;
    }
  }
  async function upload() {
    if (!connection) return;
    try {
      const files = await selectFiles('Select files to upload');
      for (const f of files) {
        const name = f.split(/[\\/]/).pop() || 'file';
        const remote =
          (currentPath === '/' ? '' : currentPath.replace(/\/$/, '')) +
          '/' +
          name;
        await queueUpload(connection.id, f, remote);
      }
      if (files.length)
        addToast('info', `${files.length} file(s) queued for upload`);
    } catch (e: any) {
      addToast('error', `Could not queue upload: ${e.message}`);
    }
  }
  async function uploadFolder() {
    if (!connection) return;
    try {
      const folder = await selectDirectory('Select folder to upload');
      if (!folder) return;
      const name =
        folder
          .replace(/[\\/]$/, '')
          .split(/[\\/]/)
          .pop() || 'folder';
      const remote =
        (currentPath === '/' ? '' : currentPath.replace(/\/$/, '')) +
        '/' +
        name;
      const jobs = await queueUploadDirectory(connection.id, folder, remote);
      addToast('info', `${jobs.length} file(s) queued for upload`);
    } catch (e: any) {
      addToast('error', `Could not queue folder upload: ${e.message}`);
    }
  }
  function refresh() {
    lastLoad = '';
    loadDirectory(connectionId, currentPath, true);
  }
  function clearIfEmpty(event: MouseEvent) {
    const target = event.target;
    if (target instanceof Element && !target.closest('tr')) selected = [];
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

<div class="view-full-height">
  <section class="page-heading files-heading">
    <div>
      <p class="eyebrow">REMOTE FILES</p>
      <h1>File Explorer</h1>
      <p class="page-subtitle">
        Browse and transfer files without changing deployment settings.
      </p>
    </div>
    <button
      class="btn btn-secondary"
      on:click={() => {
        $activityConnectionId = connection?.id ?? null;
        $activityTab = 'Transfers';
        $currentPage = 'activity';
      }}
      ><Icon name="transfer" size={15} /> View transfers{#if $transferSummary.active}<span
          class="tab-count">{$transferSummary.active}</span
        >{/if}</button
    >
  </section>
  <section class="files-panel">
    <div class="files-toolbar">
      <select
        class="form-select"
        aria-label="Connection"
        bind:value={connectionId}
        on:change={changeConnection}
        ><option value="">Select connection</option
        >{#each $connections as c}<option value={String(c.id)}>{c.name}</option
          >{/each}</select
      ><button
        class="btn btn-secondary btn-sm"
        on:click={goBack}
        aria-label="Go back"
        title="Go back"
        disabled={!back.length}><Icon name="back" /></button
      ><button
        class="btn btn-secondary btn-sm"
        on:click={goForward}
        aria-label="Go forward"
        title="Go forward"
        disabled={!forward.length}><Icon name="forward" /></button
      ><button
        class="btn btn-secondary btn-sm"
        on:click={up}
        aria-label="Go to parent directory"
        title="Go to parent directory"
        disabled={!connection || currentPath === (connection.remoteRoot || '/')}
        ><Icon name="up" /></button
      ><input
        class="form-input path-input"
        aria-label="Remote path"
        bind:value={pathInput}
        on:change={() => setPath(pathInput)}
        on:keydown={(e) => {
          if (e.key === 'Enter') setPath(pathInput);
        }}
      /><button
        class="btn btn-secondary btn-sm"
        on:click={refresh}
        aria-label="Refresh directory"
        title="Refresh directory"
        disabled={!connection}><Icon name="refresh" /></button
      >
    </div>
    <div class="files-actions">
      <span class="files-selected"
        >{selected.length
          ? `${selected.length} selected`
          : `${filtered.length} items`}</span
      >
      <input
        class="form-input files-search"
        placeholder="Filter current directory"
        bind:value={query}
      /><button
        class="btn btn-secondary btn-sm"
        on:click={upload}
        disabled={!connection}
        ><Icon name="upload" size={14} /> Upload files</button
      ><button
        class="btn btn-secondary btn-sm"
        on:click={uploadFolder}
        disabled={!connection}
        ><Icon name="upload" size={14} /> Upload folder</button
      ><button
        class="btn btn-primary btn-sm"
        on:click={() => download()}
        disabled={!selected.length || deleting}
        ><Icon name="download" size={14} /> Download</button
      ><button
        class="btn btn-danger btn-sm"
        on:click={deleteSelected}
        disabled={!selected.length || deleting}
        ><Icon name="trash" size={14} /> Delete</button
      >
    </div>
    {#if loading}<div class="files-state">
        <Icon name="loader" size={20} spin /> Loading directory…
      </div>{:else if error}<div class="files-state files-error">
        <Icon name="error" size={20} />
        <strong>Could not access directory</strong><span>{error}</span><button
          class="btn btn-secondary btn-sm"
          on:click={refresh}
          aria-label="Refresh directory"
          title="Refresh directory"
          ><Icon name="refresh" size={14} /> Retry</button
        >
      </div>{:else if !connection}<div class="files-state">
        <Icon name="server" size={24} />
        Choose a saved connection to browse remote files.
      </div>{:else if !filtered.length}<div class="files-state">
        <Icon name="folder" size={24} />
        {query ? 'No files match this filter.' : 'This directory is empty.'}
      </div>{:else}
      <!-- svelte-ignore a11y-no-noninteractive-element-interactions a11y-no-noninteractive-tabindex -->
      <div
        class="files-table-wrap"
        role="region"
        aria-label="Remote files table"
        bind:this={rows}
        tabindex="0"
        on:keydown={keydown}
        on:click={clearIfEmpty}
      >
        <table class="files-table">
          <thead
            ><tr><th>Name</th><th>Type</th><th>Size</th><th>Modified</th></tr
            ></thead
          ><tbody
            >{#each filtered as f, i (f.path)}<tr
                class:selected={selected.includes(f.path)}
                class:focused={focusIndex === i}
                on:click={(e) => choose(f, e)}
                on:dblclick={() => open(f)}
                ><td
                  ><span class="file-icon {f.isDir ? 'bg-yellow' : ''}"
                    ><Icon
                      name={getFileIcon(f.path, f.isDir)}
                      size={16}
                    /></span
                  >{f.path.split('/').pop()}</td
                ><td>{f.isDir ? 'Directory' : 'File'}</td><td
                  >{f.isDir || !f.sizeKnown ? '—' : formatSize(f.size)}</td
                ><td
                  >{f.modified
                    ? new Date(f.modified).toLocaleString()
                    : '—'}</td
                ></tr
              >{/each}</tbody
          >
        </table>
      </div>{/if}
  </section>
</div>
