<script lang="ts">
  import { onMount } from 'svelte';
  import { addToast } from '../stores';
  import { getSettings, saveSettings, selectDirectory } from '../wailsjs';
  let backupDir = '%LOCALAPPDATA%\\QuickFTP\\backups';
  let tempDir = '%TEMP%\\QuickFTP';
  let retention = 'all';
  onMount(async () => {
    try { backupDir = (await getSettings()).backupDir; } catch { backupDir = localStorage.getItem('quickftp.backupDir') ?? backupDir; }
    tempDir = localStorage.getItem('quickftp.tempDir') ?? tempDir;
    retention = localStorage.getItem('quickftp.retention') ?? retention;
  });
  async function save() {
    try { await saveSettings({ backupDir }); }
    catch (err: any) { addToast('error', `Could not save backup directory: ${err.message}`); return; }
    localStorage.setItem('quickftp.backupDir', backupDir);
    localStorage.setItem('quickftp.tempDir', tempDir);
    localStorage.setItem('quickftp.retention', retention);
    addToast('success', 'Preferences saved on this device');
  }
  async function browseBackupDir() { try { const selected = await selectDirectory('Select backup directory'); if (selected) backupDir = selected; } catch (err: any) { addToast('error', `Could not open directory picker: ${err.message}`); } }
</script>
<section class="page-heading"><div><p class="eyebrow">PREFERENCES</p><h1>Settings</h1><p class="page-subtitle">Local paths and application behavior.</p></div><button class="btn btn-primary" on:click={save}>Save settings</button></section>
<section class="settings-panel">
  <div class="settings-row"><div><strong>Default backup directory</strong><p>Where verified remote snapshots are stored.</p></div><div class="path-control settings-input"><input aria-label="Default backup directory" class="form-input" bind:value={backupDir} /><button class="btn btn-secondary" on:click={browseBackupDir}>Browse…</button></div></div>
  <div class="settings-row"><div><strong>Temporary file directory</strong><p>Used for temporary application files.</p></div><input aria-label="Temporary file directory" class="form-input settings-input" bind:value={tempDir} /></div>
  <div class="settings-row"><div><strong>Log retention</strong><p>How long deployment history should be kept.</p></div><select aria-label="Log retention" class="form-select settings-input" bind:value={retention}><option value="all">Keep all records</option><option value="90">90 days</option><option value="30">30 days</option></select></div>
  <div class="settings-row"><div><strong>QuickFTP</strong><p>Desktop deployment utility · Version 0.1.0</p></div><span class="ready-indicator"><i></i>Ready</span></div>
</section>
