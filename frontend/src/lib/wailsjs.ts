// Bridge to Wails Go backend
// During development without wails, these use mock data
// In production, these call the real Go backend via wailsjs
//
// Real imports (uncomment when wailsjs bindings are generated):
// import { GetConnections, SaveConnection, DeleteConnection, TestConnection,
//          GetProjects, SaveProject, DeleteProject,
//          StartDeploy, RollbackDeployment, GetDeploymentHistory,
//          GetPendingDeployments, GetDeploymentFiles } from '../../wailsjs/go/main/App';
// import { EventsOn } from '../../wailsjs/runtime/runtime';

import { loadProjectDeployments } from './projectHistory';
import type {
  ConnectionDTO,
  ProjectDTO,
  DeploymentDTO,
  DeploymentFileDTO,
  DeployOptions,
  RemoteFileDTO,
  TransferDTO,
} from './types';
import {
  beginDeploymentAttempt,
  updateDeploymentAttempt,
  deployLogs,
  deployPhase,
  deployStatus,
  activeDeploymentId,
  transfers,
} from './stores';

declare global {
  interface Window {
    go: any;
    runtime: any;
  }
}

// Detect if we're running inside Wails
const isWails = typeof window !== 'undefined' && window.go !== undefined;

// ============================================================
// Mock Data for standalone development
// ============================================================

let mockConnectionId = 3;
let mockProjectId = 3;
let mockDeploymentId = 5;
let mockCancelRequested = false;
let mockSettings = { backupDir: 'C:\\Users\\Public\\QuickFTP\\backups' };

const mockConnections: ConnectionDTO[] = [
  {
    id: 1,
    name: 'Production Server',
    host: 'ftp.example.com',
    port: 21,
    protocol: 'ftps_explicit',
    username: 'deploy_user',
    password: '',
    remoteRoot: '/var/www/html',
    tlsVerify: true,
    createdAt: '2024-08-15T10:30:00Z',
  },
  {
    id: 2,
    name: 'Staging Server',
    host: 'staging.example.com',
    port: 2121,
    protocol: 'ftp',
    username: 'staging_admin',
    password: '',
    remoteRoot: '/wwwroot',
    tlsVerify: false,
    createdAt: '2024-09-01T14:20:00Z',
  },
  {
    id: 3,
    name: 'Dev Environment',
    host: '192.168.1.50',
    port: 21,
    protocol: 'ftp',
    username: 'devuser',
    password: '',
    remoteRoot: '/',
    tlsVerify: false,
    createdAt: '2024-10-05T09:00:00Z',
  },
];

const mockProjects: ProjectDTO[] = [
  {
    id: 1,
    name: 'Customer Portal',
    projectType: 'angular',
    localPath: 'C:\\Projects\\customer-portal',
    buildCommand: 'ng build --configuration production',
    outputRelPath: 'dist/customer-portal',
    artifactRulesJson: '',
  },
  {
    id: 2,
    name: 'Admin API',
    projectType: 'dotnet',
    localPath: 'C:\\Projects\\admin-api',
    buildCommand: 'dotnet publish -c Release',
    outputRelPath: 'bin/Release/net8.0/publish',
    artifactRulesJson: '{"exclude": ["appsettings.Development.json"]}',
  },
  {
    id: 3,
    name: 'Marketing Site',
    projectType: 'angular',
    localPath: 'C:\\Projects\\marketing-site',
    buildCommand: 'npm run build',
    outputRelPath: 'dist/marketing',
    artifactRulesJson: '',
  },
];

const mockDeployments: DeploymentDTO[] = [
  {
    id: 1,
    projectId: 1,
    projectName: 'Customer Portal',
    projectType: 'angular',
    connectionId: 1,
    connectionName: 'Production Server',
    connectionHost: 'ftp.example.com',
    remoteRoot: '/var/www/html',
    buildCommand: 'ng build --configuration production',
    outputRelPath: 'dist/customer-portal',
    status: 'completed',
    currentPhase: 'done',
    backupDir: '/backups/20241010_143022',
    errorMessage: '',
    startedAt: '2024-10-10T14:30:22Z',
    finishedAt: '2024-10-10T14:32:45Z',
  },
  {
    id: 2,
    projectId: 2,
    projectName: 'Admin API',
    projectType: 'dotnet',
    connectionId: 1,
    connectionName: 'Production Server',
    connectionHost: 'ftp.example.com',
    remoteRoot: '/var/www/html',
    buildCommand: 'dotnet publish -c Release',
    outputRelPath: 'bin/Release/net8.0/publish',
    status: 'failed',
    currentPhase: 'upload',
    backupDir: '/backups/20241011_091000',
    errorMessage: 'Connection timed out during file upload',
    startedAt: '2024-10-11T09:10:00Z',
    finishedAt: '2024-10-11T09:12:34Z',
  },
  {
    id: 3,
    projectId: 1,
    projectName: 'Customer Portal',
    projectType: 'angular',
    connectionId: 2,
    connectionName: 'Staging Server',
    connectionHost: 'staging.example.com',
    remoteRoot: '/wwwroot',
    buildCommand: 'ng build --configuration production',
    outputRelPath: 'dist/customer-portal',
    status: 'completed',
    currentPhase: 'done',
    backupDir: '/backups/20241012_160000',
    errorMessage: '',
    startedAt: '2024-10-12T16:00:00Z',
    finishedAt: '2024-10-12T16:03:12Z',
  },
  {
    id: 4,
    projectId: 3,
    projectName: 'Marketing Site',
    projectType: 'angular',
    connectionId: 2,
    connectionName: 'Staging Server',
    connectionHost: 'staging.example.com',
    remoteRoot: '/wwwroot',
    buildCommand: 'npm run build',
    outputRelPath: 'dist/marketing',
    status: 'rolled_back',
    currentPhase: 'rollback',
    backupDir: '/backups/20241013_110500',
    errorMessage: 'Build failed: exit code 1',
    startedAt: '2024-10-13T11:05:00Z',
    finishedAt: '2024-10-13T11:06:20Z',
  },
  {
    id: 5,
    projectId: 2,
    projectName: 'Admin API',
    projectType: 'dotnet',
    connectionId: 1,
    connectionName: 'Production Server',
    connectionHost: 'ftp.example.com',
    remoteRoot: '/var/www/html',
    buildCommand: 'dotnet publish -c Release',
    outputRelPath: 'bin/Release/net8.0/publish',
    status: 'completed',
    currentPhase: 'done',
    backupDir: '/backups/20241014_080000',
    errorMessage: '',
    startedAt: '2024-10-14T08:00:00Z',
    finishedAt: '2024-10-14T08:04:10Z',
  },
];

const mockDeploymentFiles: DeploymentFileDTO[] = [
  {
    id: 1,
    deploymentId: 1,
    relativePath: 'index.html',
    stage: 'upload',
    status: 'verified',
    sizeBytes: 4520,
    sha256Local: '',
    errorDetail: '',
    updatedAt: '',
  },
  {
    id: 2,
    deploymentId: 1,
    relativePath: 'main.js',
    stage: 'upload',
    status: 'verified',
    sizeBytes: 245800,
    sha256Local: '',
    errorDetail: '',
    updatedAt: '',
  },
  {
    id: 3,
    deploymentId: 1,
    relativePath: 'styles.css',
    stage: 'upload',
    status: 'verified',
    sizeBytes: 18300,
    sha256Local: '',
    errorDetail: '',
    updatedAt: '',
  },
  {
    id: 4,
    deploymentId: 1,
    relativePath: 'assets/logo.png',
    stage: 'upload',
    status: 'verified',
    sizeBytes: 12400,
    sha256Local: '',
    errorDetail: '',
    updatedAt: '',
  },
];

const mockPendingDeployments: DeploymentDTO[] = [];

// Simulated delay for mock calls
function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// ============================================================
// Connection API
// ============================================================

export async function getConnections(): Promise<ConnectionDTO[]> {
  if (isWails) {
    return await window.go.main.App.GetConnections();
  }
  await delay(200);
  return [...mockConnections];
}

export async function getSettings(): Promise<{
  backupDir: string;
  downloadDir: string;
  downloadConflict: string;
  uploadConflict: string;
  maxConcurrent: number;
}> {
  if (isWails) return await window.go.main.App.GetSettings();
  await delay(80);
  return {
    ...mockSettings,
    downloadDir: '',
    downloadConflict: 'Ask',
    uploadConflict: 'Ask',
    maxConcurrent: 2,
  };
}
export async function saveSettings(settings: {
  backupDir: string;
  downloadDir?: string;
  downloadConflict?: string;
  uploadConflict?: string;
  maxConcurrent?: number;
}): Promise<void> {
  if (isWails) return await window.go.main.App.SaveSettings(settings);
  await delay(100);
  mockSettings = { ...settings };
}
export async function selectDirectory(title: string): Promise<string> {
  if (!isWails) return '';
  return await window.go.main.App.SelectDirectory(title);
}

export async function listRemoteDirectory(
  connectionId: number,
  remotePath: string
): Promise<RemoteFileDTO[]> {
  if (!isWails)
    throw new Error('Remote browsing requires the QuickFTP desktop app');
  return await window.go.main.App.ListRemoteDirectory(connectionId, remotePath);
}

export interface RemoteDeleteItem {
  path: string;
  isDir: boolean;
}

export async function deleteRemoteEntries(
  connectionId: number,
  items: RemoteDeleteItem[]
): Promise<void> {
  if (!isWails)
    throw new Error('Remote file deletion requires the QuickFTP desktop app');
  return await window.go.main.App.DeleteRemoteEntries(connectionId, items);
}

export async function deleteRemoteEntry(
  connectionId: number,
  remotePath: string,
  isDir: boolean
): Promise<void> {
  if (!isWails)
    throw new Error('Remote file deletion requires the QuickFTP desktop app');
  return await window.go.main.App.DeleteRemoteEntry(
    connectionId,
    remotePath,
    isDir
  );
}
export async function queueDownload(
  connectionId: number,
  remotePath: string,
  localPath: string,
  size: number
): Promise<TransferDTO> {
  if (!isWails) throw new Error('Transfers require the QuickFTP desktop app');
  return await window.go.main.App.QueueDownload(
    connectionId,
    remotePath,
    localPath,
    size
  );
}
export async function queueUpload(
  connectionId: number,
  localPath: string,
  remotePath: string
): Promise<TransferDTO> {
  if (!isWails) throw new Error('Transfers require the QuickFTP desktop app');
  return await window.go.main.App.QueueUpload(
    connectionId,
    localPath,
    remotePath
  );
}
export async function queueDownloadDirectory(
  connectionId: number,
  remotePath: string
): Promise<TransferDTO[]> {
  if (!isWails) throw new Error('Transfers require the QuickFTP desktop app');
  return await window.go.main.App.QueueDownloadDirectory(
    connectionId,
    remotePath
  );
}
export async function queueUploadDirectory(
  connectionId: number,
  localPath: string,
  remotePath: string
): Promise<TransferDTO[]> {
  if (!isWails) throw new Error('Transfers require the QuickFTP desktop app');
  return await window.go.main.App.QueueUploadDirectory(
    connectionId,
    localPath,
    remotePath
  );
}
export async function selectFiles(title: string): Promise<string[]> {
  if (!isWails) return [];
  return await window.go.main.App.SelectFiles(title);
}
export async function getTransfers(): Promise<TransferDTO[]> {
  if (!isWails) return [];
  return (await window.go.main.App.GetTransfers()) || [];
}
export async function clearTransferHistory(connectionId = 0): Promise<void> {
  const removed: string[] = isWails
    ? await window.go.main.App.ClearTransferHistory(connectionId)
    : [];
  const ids = new Set(removed);
  transfers.update((items) => items.filter((item) => !ids.has(item.id)));
}
export async function getDeploymentLogs(
  id: number,
  offset = 0,
  limit = 50
): Promise<{
  entries: { id: number; stream: string; line: string; createdAt: string }[];
  total: number;
}> {
  if (!isWails) return { entries: [], total: 0 };
  return await window.go.main.App.GetDeploymentLogs(id, offset, limit);
}
export async function clearDeploymentLogs(id: number): Promise<void> {
  if (isWails) await window.go.main.App.ClearDeploymentLogs(id);
}

export async function cancelTransfer(id: string): Promise<void> {
  if (isWails) return await window.go.main.App.CancelTransfer(id);
}
export async function resolveTransferConflict(
  id: string,
  policy: string
): Promise<void> {
  if (isWails)
    return await window.go.main.App.ResolveTransferConflict(id, policy);
}
export async function retryTransfer(id: string): Promise<void> {
  if (isWails) await window.go.main.App.RetryTransfer(id);
  transfers.update((items) => items.filter((item) => item.id !== id));
}
export async function removeTransfer(id: string): Promise<void> {
  if (isWails) await window.go.main.App.RemoveTransfer(id);
  transfers.update((xs) => xs.filter((x) => x.id !== id));
}

export async function saveConnection(dto: ConnectionDTO): Promise<void> {
  if (isWails) {
    return await window.go.main.App.SaveConnection(dto);
  }
  await delay(300);
  if (dto.id === 0) {
    dto.id = ++mockConnectionId;
    dto.createdAt = new Date().toISOString();
    mockConnections.push({ ...dto });
  } else {
    const idx = mockConnections.findIndex((c) => c.id === dto.id);
    if (idx >= 0) mockConnections[idx] = { ...dto };
  }
}

export async function deleteConnection(id: number): Promise<void> {
  if (isWails) {
    return await window.go.main.App.DeleteConnection(id);
  }
  await delay(200);
  const idx = mockConnections.findIndex((c) => c.id === id);
  if (idx >= 0) mockConnections.splice(idx, 1);
}

export async function testConnection(dto: ConnectionDTO): Promise<void> {
  if (isWails) {
    return await window.go.main.App.TestConnection(dto);
  }
  await delay(1500);
  // Mock: randomly succeed or fail for realism
  if (Math.random() > 0.3) return;
  throw new Error('Mock: Connection refused — verify host and port');
}

// ============================================================
// Project API
// ============================================================

export async function getProjects(): Promise<ProjectDTO[]> {
  if (isWails) {
    return await window.go.main.App.GetProjects();
  }
  await delay(200);
  return [...mockProjects];
}

export async function saveProject(dto: ProjectDTO): Promise<void> {
  if (isWails) {
    return await window.go.main.App.SaveProject(dto);
  }
  await delay(300);
  if (dto.id === 0) {
    dto.id = ++mockProjectId;
    mockProjects.push({ ...dto });
  } else {
    const idx = mockProjects.findIndex((p) => p.id === dto.id);
    if (idx >= 0) mockProjects[idx] = { ...dto };
  }
}

export async function deleteProject(id: number): Promise<void> {
  if (isWails) {
    return await window.go.main.App.DeleteProject(id);
  }
  await delay(200);
  const idx = mockProjects.findIndex((p) => p.id === id);
  if (idx >= 0) mockProjects.splice(idx, 1);
}

// ============================================================
// Deployment API
// ============================================================

export async function startDeploy(
  projectName: string,
  connectionName: string,
  opts: DeployOptions
): Promise<void> {
  beginDeploymentAttempt(projectName, connectionName);
  if (isWails) {
    try {
      await window.go.main.App.StartDeploy(projectName, connectionName, opts);
    } catch (error: any) {
      deployStatus.set('failed');
      updateDeploymentAttempt({
        status: 'failed',
        errorMessage: error?.message || String(error),
        finishedAt: new Date().toISOString(),
      });
      throw error;
    }
    return;
  }
  void simulateMockDeployment(projectName, connectionName);
}

async function simulateMockDeployment(
  projectName: string,
  connectionName: string
) {
  // Mock deployment simulation
  const deployId = ++mockDeploymentId;
  mockCancelRequested = false;
  activeDeploymentId.set(deployId);
  updateDeploymentAttempt({ id: deployId });
  deployStatus.set('running');
  deployLogs.set([]);
  deployPhase.set('build');

  const phases = ['build', 'backup', 'upload', 'verify', 'done'];
  const logLines = [
    { stream: 'stdout', line: `[deploy] Starting deployment #${deployId}` },
    {
      stream: 'stdout',
      line: `[deploy] Project: ${projectName} → ${connectionName}`,
    },
    { stream: 'stdout', line: '[build] Running build command...' },
    { stream: 'stdout', line: '[build] Compiling TypeScript...' },
    { stream: 'stdout', line: '[build] Bundling with Webpack...' },
    { stream: 'stdout', line: '[build] Build completed successfully (23.4s)' },
    { stream: 'stdout', line: '[backup] Creating remote backup...' },
    {
      stream: 'stdout',
      line: '[backup] Backed up 142 files to /backups/20241015',
    },
    { stream: 'stdout', line: '[upload] Uploading 38 files...' },
    { stream: 'stdout', line: '[upload]   index.html (4.5 KB)' },
    { stream: 'stdout', line: '[upload]   main.abc123.js (245.8 KB)' },
    { stream: 'stdout', line: '[upload]   styles.def456.css (18.3 KB)' },
    { stream: 'stdout', line: '[upload]   assets/logo.png (12.4 KB)' },
    { stream: 'stdout', line: '[upload] Upload complete (38/38 files)' },
    { stream: 'stdout', line: '[verify] Verifying deployed files...' },
    { stream: 'stdout', line: '[verify] All checksums match' },
    {
      stream: 'stdout',
      line: `[deploy] Deployment #${deployId} completed successfully ✓`,
    },
  ];

  let phaseIdx = 0;
  for (let i = 0; i < logLines.length; i++) {
    await delay(400 + Math.random() * 300);
    if (mockCancelRequested) {
      deployLogs.update((logs) => [
        ...logs,
        {
          stream: 'stderr',
          line: '[deploy] Cancelled after the current operation',
        },
      ]);
      deployStatus.set('cancelled');
      updateDeploymentAttempt({
        status: 'cancelled',
        cancelled: true,
        finishedAt: new Date().toISOString(),
      });
      mockDeployments.unshift({
        id: deployId,
        projectId: mockProjects.find((p) => p.name === projectName)?.id ?? 0,
        connectionId:
          mockConnections.find((c) => c.name === connectionName)?.id ?? 0,
        status: 'cancelled',
        currentPhase: 'upload',
        backupDir: '/backups/mock-cancelled',
        errorMessage: 'Deployment cancelled by user',
        startedAt: new Date().toISOString(),
        finishedAt: new Date().toISOString(),
      });
      return;
    }
    deployLogs.update((logs) => [...logs, logLines[i]]);

    // Advance phase
    if (i === 5 && phaseIdx < phases.length) {
      phaseIdx = 1;
      deployPhase.set(phases[phaseIdx]);
    }
    if (i === 7 && phaseIdx < phases.length) {
      phaseIdx = 2;
      deployPhase.set(phases[phaseIdx]);
    }
    if (i === 13 && phaseIdx < phases.length) {
      phaseIdx = 3;
      deployPhase.set(phases[phaseIdx]);
    }
    if (i === 15 && phaseIdx < phases.length) {
      phaseIdx = 4;
      deployPhase.set(phases[phaseIdx]);
    }
  }

  deployStatus.set('completed');
  updateDeploymentAttempt({
    status: 'completed',
    currentPhase: 'done',
    finishedAt: new Date().toISOString(),
  });

  mockDeployments.unshift({
    id: deployId,
    projectId: mockProjects.find((p) => p.name === projectName)?.id ?? 0,
    connectionId:
      mockConnections.find((c) => c.name === connectionName)?.id ?? 0,
    status: 'completed',
    currentPhase: 'done',
    backupDir: '/backups/20241015_mock',
    errorMessage: '',
    startedAt: new Date().toISOString(),
    finishedAt: new Date().toISOString(),
  });
}

export async function cancelDeploy(): Promise<void> {
  if (isWails) return await window.go.main.App.CancelDeploy();
  mockCancelRequested = true;
}

export async function rollbackDeployment(
  deploymentId: number,
  connectionName: string
): Promise<void> {
  if (isWails) {
    return await window.go.main.App.RollbackDeployment(
      deploymentId,
      connectionName
    );
  }
  await delay(2000);
  const dep = mockDeployments.find((d) => d.id === deploymentId);
  if (dep) dep.status = 'rolled_back';
}

export async function getProjectDeployments(
  projectId: number,
  limit = 200
): Promise<DeploymentDTO[]> {
  if (isWails)
    return loadProjectDeployments(window.go.main.App, projectId, limit);
  return mockDeployments
    .filter((run) => run.projectId === projectId)
    .slice(0, limit);
}
export async function getDeploymentHistory(
  limit: number
): Promise<DeploymentDTO[]> {
  if (isWails) {
    return await window.go.main.App.GetDeploymentHistory(limit);
  }
  await delay(200);
  return [...mockDeployments].slice(0, limit);
}

export async function getDeployment(id: number): Promise<DeploymentDTO | null> {
  if (isWails) {
    return await window.go.main.App.GetDeployment(id);
  }
  await delay(100);
  const dep = mockDeployments.find((d) => d.id === id);
  return dep ? { ...dep } : null;
}

export async function getPendingDeployments(): Promise<DeploymentDTO[]> {
  if (isWails) {
    return await window.go.main.App.GetPendingDeployments();
  }
  await delay(100);
  return [...mockPendingDeployments];
}

export async function getDeploymentFiles(
  deploymentId: number,
  stage: string
): Promise<DeploymentFileDTO[]> {
  if (isWails) {
    return await window.go.main.App.GetDeploymentFiles(deploymentId, stage);
  }
  await delay(200);
  return mockDeploymentFiles.filter((f) => f.deploymentId === deploymentId);
}

// ============================================================
// Event System
// ============================================================

type EventCallback = (data: any) => void;
const eventListeners: Map<string, EventCallback[]> = new Map();

export function onEvent(
  eventName: string,
  callback: EventCallback
): () => void {
  if (isWails && window.runtime?.EventsOn) {
    // Use real Wails event system
    return window.runtime.EventsOn(eventName, callback);
  }

  // Mock event system
  if (!eventListeners.has(eventName)) {
    eventListeners.set(eventName, []);
  }
  eventListeners.get(eventName)!.push(callback);

  return () => {
    const listeners = eventListeners.get(eventName);
    if (listeners) {
      const idx = listeners.indexOf(callback);
      if (idx >= 0) listeners.splice(idx, 1);
    }
  };
}

// Helper to look up names by ID (used in UI display)
export function getConnectionNameById(id: number): string {
  const conn = mockConnections.find((c) => c.id === id);
  return conn?.name ?? `Connection #${id}`;
}

export function getProjectNameById(id: number): string {
  const proj = mockProjects.find((p) => p.id === id);
  return proj?.name ?? `Project #${id}`;
}

export async function exportData(): Promise<boolean> {
  if (!isWails)
    throw new Error('Data export requires the QuickFTP desktop app');
  return await window.go.main.App.ExportData();
}

export async function importData(): Promise<{
  connections: number;
  projects: number;
  skipped: number;
} | null> {
  if (!isWails)
    throw new Error('Data import requires the QuickFTP desktop app');
  return await window.go.main.App.ImportData();
}
