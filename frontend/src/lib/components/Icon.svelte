<script context="module" lang="ts">
  export type { IconName } from '../icons';
</script>

<script lang="ts">
  import type { IconName } from '../icons';
  import {
    Archive,
    ArrowLeft,
    ArrowRight,
    ArrowUp,
    ArrowUpDown,
    ArrowUpRight,
    Binary,
    Boxes,
    Check,
    ChevronDown,
    ChevronRight,
    ChevronUp,
    CircleCheck,
    CircleX,
    Clock3,
    CloudUpload,
    Code,
    Copy,
    Database,
    Download,
    File,
    FileArchive,
    FileCode,
    FileImage,
    FileText,
    Files,
    Folder,
    FolderKanban,
    FolderTree,
    FolderUp,
    History,
    Info,
    LayoutDashboard,
    Loader,
    Pencil,
    Play,
    Plus,
    RefreshCw,
    Rocket,
    RotateCcw,
    Search,
    Server,
    Settings,
    Trash2,
    TriangleAlert,
    Upload,
    X,
  } from 'lucide-svelte';

  export let name: IconName;
  export let size = 16;
  export let title = '';
  export let spin = false;

  const icons: Record<IconName, typeof File> = {
    back: ArrowLeft,
    forward: ArrowRight,
    up: ArrowUp,
    arrowUpRight: ArrowUpRight,
    chevronDown: ChevronDown,
    chevronUp: ChevronUp,
    chevronRight: ChevronRight,
    check: Check,
    success: CircleCheck,
    error: CircleX,
    warning: TriangleAlert,
    info: Info,
    close: X,
    clock: Clock3,
    loader: Loader,
    download: Download,
    upload: Upload,
    cloudUpload: CloudUpload,
    file: File,
    fileCode: FileCode,
    fileArchive: FileArchive,
    fileImage: FileImage,
    fileText: FileText,
    binary: Binary,
    database: Database,
    archive: Archive,
    folder: Folder,
    folderTree: FolderTree,
    folderUp: FolderUp,
    files: Files,
    play: Play,
    refresh: RefreshCw,
    rollback: RotateCcw,
    dashboard: LayoutDashboard,
    server: Server,
    projects: FolderKanban,
    settings: Settings,
    history: History,
    plus: Plus,
    search: Search,
    edit: Pencil,
    trash: Trash2,
    copy: Copy,
    angular: Code,
    dotnet: Boxes,
    deploy: Rocket,
    transfer: ArrowUpDown,
  };

  $: IconComponent = icons[name] || File;
</script>

<span
  class="lucide-icon-wrap"
  class:spin-anim={spin || name === 'loader'}
  style="display: inline-flex; align-items: center; justify-content: center; line-height: 0;"
>
  <svelte:component
    this={IconComponent}
    {size}
    strokeWidth={1.8}
    aria-hidden={title ? undefined : 'true'}
    aria-label={title || undefined}
  />
</span>

<style>
  .spin-anim {
    animation: lucide-spin 1.5s linear infinite;
  }
  @keyframes lucide-spin {
    from {
      transform: rotate(0deg);
    }
    to {
      transform: rotate(360deg);
    }
  }
</style>
