<script lang="ts">
  import Icon from './Icon.svelte';
  import { createEventDispatcher } from 'svelte';
  import type { ProjectDTO } from '../types';
  import { saveProject, selectDirectory } from '../wailsjs';
  import { addToast } from '../stores';

  export let project: ProjectDTO | null = null;

  const dispatch = createEventDispatcher();

  let name = project ? project.name : '';
  let projectType = project ? project.projectType : 'angular';
  let localPath = project ? project.localPath : '';
  let buildCommand = project
    ? project.buildCommand
    : projectType === 'angular'
      ? 'npm run build'
      : 'dotnet publish -c Release';
  let outputRelPath = project
    ? project.outputRelPath
    : projectType === 'angular'
      ? 'dist'
      : 'bin/Release/net8.0/publish';
  let artifactRulesJson = project ? project.artifactRulesJson : '';
  let activeTab = 'general';
  let includeText = '';
  let excludeText = '';

  $: if (artifactRulesJson) {
    try {
      const parsed = JSON.parse(artifactRulesJson || '{}');
      includeText = Array.isArray(parsed.include)
        ? parsed.include.join('\n')
        : '';
      excludeText = Array.isArray(parsed.exclude)
        ? parsed.exclude.join('\n')
        : '';
    } catch {
      /* validation reports malformed rules on save */
    }
  }

  let saving = false;
  let errorMsg = '';

  function handleTypeChange() {
    if (!project) {
      if (projectType === 'angular') {
        buildCommand = 'npm run build';
        outputRelPath = 'dist';
      } else {
        buildCommand = 'dotnet publish -c Release';
        outputRelPath = 'bin/Release/net8.0/publish';
      }
    }
  }

  async function handleSave() {
    if (!name.trim()) {
      errorMsg = 'Project name is required';
      return;
    }
    if (!localPath.trim()) {
      errorMsg = 'Local path is required';
      return;
    }
    if (!outputRelPath.trim()) {
      errorMsg = 'Output relative path is required';
      return;
    }
    if (projectType === 'dotnet' && !includeText.trim()) {
      activeTab = 'files';
      errorMsg =
        'For .NET projects, add the DLLs and runtime paths to the include list so only selected publish files are uploaded.';
      return;
    }

    artifactRulesJson = JSON.stringify({
      include: includeText
        .split(/\r?\n/)
        .map((x) => x.trim())
        .filter(Boolean),
      exclude: excludeText
        .split(/\r?\n/)
        .map((x) => x.trim())
        .filter(Boolean),
    });
    if (artifactRulesJson.trim()) {
      try {
        JSON.parse(artifactRulesJson);
      } catch (err: any) {
        errorMsg =
          'Artifact rules must be valid JSON (e.g. {"include":[], "exclude":[]})';
        return;
      }
    }

    saving = true;
    errorMsg = '';
    try {
      await saveProject({
        id: project ? project.id : 0,
        name: name.trim(),
        projectType,
        localPath: localPath.trim(),
        buildCommand: buildCommand.trim(),
        outputRelPath: outputRelPath.trim(),
        artifactRulesJson: artifactRulesJson.trim() || '{}',
      });
      addToast('success', `Project "${name}" saved successfully`);
      dispatch('saved');
    } catch (err: any) {
      errorMsg = err.message || 'Failed to save project';
      addToast('error', errorMsg);
    } finally {
      saving = false;
    }
  }

  function handleCancel() {
    dispatch('cancel');
  }

  async function chooseProjectDirectory() {
    try {
      const selected = await selectDirectory('Select project directory');
      if (selected) localPath = selected;
    } catch (err: any) {
      addToast('error', `Could not open directory picker: ${err.message}`);
    }
  }
</script>

<div class="modal-overlay">
  <div class="modal project-modal">
    <div class="modal-header">
      <h3>{project ? 'Edit Project' : 'New Project'}</h3>
      <button class="modal-close" on:click={handleCancel} aria-label="Close dialog" title="Close"><Icon name="close" size={16} /></button>
    </div>

    <div class="project-tabs" role="tablist" aria-label="Project settings">
      {#each [{ id: 'general', label: 'General' }, { id: 'build', label: 'Build / Publish' }, { id: 'files', label: 'File Selection' }, { id: 'deployment', label: 'Deployment' }, { id: 'backup', label: 'Backup' }] as tab}
        <button
          class="project-tab"
          class:active={activeTab === tab.id}
          role="tab"
          aria-selected={activeTab === tab.id}
          on:click={() => (activeTab = tab.id)}>{tab.label}</button
        >
      {/each}
    </div>
    <div class="modal-body">
      {#if errorMsg}
        <div
          style="background: var(--color-danger-light); color: var(--color-danger); padding: 8px 12px; border-radius: var(--radius-sm); margin-bottom: 16px; font-size: 13px;"
        >
          {errorMsg}
        </div>
      {/if}

      {#if activeTab === 'general'}
        <div class="form-group">
          <label class="form-label" for="proj-name">Project Name</label>
          <input
            id="proj-name"
            class="form-input"
            type="text"
            bind:value={name}
            placeholder="e.g. Customer Portal"
          />
        </div>

        <div class="form-group">
          <label class="form-label" for="proj-type"
            >Project Framework / Type</label
          >
          <select
            id="proj-type"
            class="form-select"
            bind:value={projectType}
            on:change={handleTypeChange}
          >
            <option value="angular">Angular (Node.js / npm)</option>
            <option value="dotnet">.NET / ASP.NET Core</option>
          </select>
        </div>

        <div class="form-group">
          <label class="form-label" for="proj-path"
            >Local Project Directory Path</label
          >
          <div class="path-control">
            <input
              id="proj-path"
              class="form-input"
              type="text"
              bind:value={localPath}
              placeholder="C:\Projects\my-app"
            /><button
              class="btn btn-secondary"
              type="button"
              on:click={chooseProjectDirectory}><Icon name="folder" size={14} /> Browse…</button
            >
          </div>
          <div class="form-hint">
            Absolute local path containing the project sources
          </div>
        </div>
      {:else if activeTab === 'build'}
        <div class="form-group">
          <label class="form-label" for="proj-cmd">Build Command</label>
          <input
            id="proj-cmd"
            class="form-input"
            type="text"
            bind:value={buildCommand}
            placeholder="e.g. npm run build"
          />
          <div class="form-hint">
            Command executed inside the project directory before artifacts are
            collected (leave blank to skip build)
          </div>
        </div>

        <div class="form-group">
          <label class="form-label" for="proj-out">Output Relative Path</label>
          <input
            id="proj-out"
            class="form-input"
            type="text"
            bind:value={outputRelPath}
            placeholder="dist/browser or bin/Release/net8.0/publish"
          />
          <div class="form-hint">
            Path relative to project root containing the compiled files to
            upload
          </div>
        </div>
      {:else if activeTab === 'files'}
        <div class="form-row">
          <div class="form-group">
            <label class="form-label" for="proj-include"
              >Include files and patterns</label
            ><textarea
              id="proj-include"
              class="form-textarea rule-editor"
              bind:value={includeText}
              rows="8"
              placeholder={'*.dll\nruntime/**\nwwwroot/**'}
            ></textarea>
            <div class="form-hint">
              One relative path or glob per line. .NET projects should list the
              DLLs to publish.
            </div>
          </div>
          <div class="form-group">
            <label class="form-label" for="proj-exclude">Exclude patterns</label
            ><textarea
              id="proj-exclude"
              class="form-textarea rule-editor"
              bind:value={excludeText}
              rows="8"
              placeholder={'*.pdb\nappsettings.Development.json'}
            ></textarea>
            <div class="form-hint">Excluded files are never uploaded.</div>
          </div>
        </div>
        <div class="selection-note">
          <strong>Artifact source</strong><span
            >Files are selected from <code
              >{outputRelPath || 'your configured output directory'}</code
            >.</span
          >
        </div>
      {:else if activeTab === 'deployment'}
        <div class="selection-note">
          <strong>Remote destination</strong><span
            >The active connection’s remote root is used as the deployment
            destination.</span
          >
        </div>
        <div class="selection-note">
          <strong>Deployment order</strong><span
            >QuickFTP snapshots and verifies remote files, stages and verifies
            uploads, then removes stale remote files.</span
          >
        </div>
      {:else}
        <div class="selection-note safety-note">
          <strong>Backup safety</strong><span
            >Every remote file is downloaded and verified before QuickFTP may
            delete or replace it. Backups are stored in the application backup
            directory.</span
          >
        </div>
      {/if}

      <div class="form-actions">
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
          disabled={saving}
        >
          {saving ? 'Saving...' : 'Save Project'}
        </button>
      </div>
    </div>
  </div>
</div>
