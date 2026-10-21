export type IconName =
  | 'back'
  | 'forward'
  | 'up'
  | 'arrowUpRight'
  | 'chevronDown'
  | 'chevronUp'
  | 'chevronRight'
  | 'check'
  | 'success'
  | 'error'
  | 'warning'
  | 'info'
  | 'close'
  | 'clock'
  | 'loader'
  | 'download'
  | 'upload'
  | 'cloudUpload'
  | 'file'
  | 'fileCode'
  | 'fileArchive'
  | 'fileImage'
  | 'fileText'
  | 'binary'
  | 'database'
  | 'archive'
  | 'folder'
  | 'folderTree'
  | 'folderUp'
  | 'files'
  | 'play'
  | 'refresh'
  | 'rollback'
  | 'dashboard'
  | 'server'
  | 'projects'
  | 'settings'
  | 'history'
  | 'plus'
  | 'search'
  | 'edit'
  | 'trash'
  | 'copy'
  | 'angular'
  | 'dotnet'
  | 'deploy'
  | 'transfer';

/**
 * Returns a semantic icon name based on file extension and directory status.
 */
export function getFileIcon(filePath: string, isDir: boolean): IconName {
  if (isDir) return 'folder';
  const name = filePath.split('/').pop() || '';
  const ext = name.includes('.') ? name.split('.').pop()?.toLowerCase() || '' : '';

  switch (ext) {
    case 'dll':
    case 'exe':
    case 'bin':
    case 'so':
    case 'dylib':
      return 'binary';
    case 'bak':
    case 'db':
    case 'sqlite':
    case 'sqlite3':
    case 'sql':
      return 'database';
    case 'zip':
    case 'tar':
    case 'gz':
    case 'tgz':
    case '7z':
    case 'rar':
      return 'archive';
    case 'js':
    case 'mjs':
    case 'cjs':
    case 'ts':
    case 'cs':
    case 'html':
    case 'htm':
    case 'css':
    case 'scss':
    case 'json':
    case 'xml':
    case 'yaml':
    case 'yml':
      return 'fileCode';
    case 'png':
    case 'jpg':
    case 'jpeg':
    case 'gif':
    case 'svg':
    case 'ico':
    case 'webp':
      return 'fileImage';
    case 'txt':
    case 'md':
    case 'log':
      return 'fileText';
    default:
      return 'file';
  }
}

/**
 * Returns a semantic icon name for a project technology.
 */
export function getProjectIcon(projectType: string): IconName {
  switch (projectType.toLowerCase()) {
    case 'angular':
      return 'angular';
    case 'dotnet':
      return 'dotnet';
    default:
      return 'projects';
  }
}

/**
 * Returns a semantic icon name for a deployment/connection status.
 */
export function getStatusIcon(status: string, cancelled = false): IconName {
  if (cancelled || status === 'cancelled') return 'warning';
  switch (status.toLowerCase()) {
    case 'completed':
    case 'success':
    case 'connected':
      return 'success';
    case 'failed':
    case 'error':
      return 'error';
    case 'running':
    case 'checking':
    case 'uploading':
    case 'downloading':
      return 'loader';
    case 'rolled_back':
      return 'rollback';
    default:
      return 'clock';
  }
}

/**
 * Returns a semantic icon name for transfer direction.
 */
export function getDirectionIcon(direction: string): IconName {
  const dir = direction.toLowerCase();
  if (dir.includes('upload')) return 'upload';
  if (dir.includes('download')) return 'download';
  return 'arrowUpRight';
}

