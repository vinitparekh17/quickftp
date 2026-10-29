import { derived, get, writable } from 'svelte/store';
import type {
  Page,
  ConnectionDTO,
  ProjectDTO,
  DeploymentDTO,
  ToastMessage,
  DeployLog,
} from './types';

// Navigation
export const currentPage = writable<Page>('dashboard');
export const selectedProjectName = writable<string>('');
export const selectedConnectionName = writable<string>('');
export const newProjectRequested = writable<boolean>(false);

// Data stores
export const connections = writable<ConnectionDTO[]>([]);
export const projects = writable<ProjectDTO[]>([]);
export const deploymentHistory = writable<DeploymentDTO[]>([]);
export const pendingDeployments = writable<DeploymentDTO[]>([]);
export const transfers = writable<import('./types').TransferDTO[]>([]);
export const activityLogs = writable<
  { timestamp: string; level: string; category: string; message: string }[]
>([]);
export const activityTab = writable<'Deployments' | 'Transfers' | 'Logs'>(
  'Deployments'
);
export const manualTransfers = derived(transfers, (items) =>
  items.filter((item) => item.direction !== 'Deployment upload')
);
export const transferSummary = derived(manualTransfers, (items) => {
  const batchKey = (item: import('./types').TransferDTO) =>
    item.batchId
      ? `${item.connectionId}:${item.batchId}`
      : `${item.connectionId}:${item.direction}:${item.startedAt?.slice(0, 10) || 'unknown date'}`;
  const activeBatches = new Set(
    items
      .filter((item) => ['Queued', 'Running', 'Waiting'].includes(item.status))
      .map(batchKey)
  );
  const attentionBatches = new Set(
    items
      .filter((item) =>
        ['Waiting', 'Failed', 'Interrupted'].includes(item.status)
      )
      .map(batchKey)
  );
  return {
    active: items.filter((item) =>
      ['Queued', 'Running', 'Waiting'].includes(item.status)
    ).length,
    attention: items.filter((item) =>
      ['Waiting', 'Failed', 'Interrupted'].includes(item.status)
    ).length,
    activeBatches: activeBatches.size,
    attentionBatches: attentionBatches.size,
  };
});
export const deploymentProjectId = writable<number | null>(null);
export const activityConnectionId = writable<number | null>(null);
export const requestedDeploymentId = writable<number | null>(null);
export const remotePathByConnection = writable<Record<number, string>>({});

// Deploy state
export const deployLogs = writable<DeployLog[]>([]);
export const deployPhase = writable<string>('');
export const deployStatus = writable<
  'idle' | 'running' | 'completed' | 'failed' | 'cancelled'
>('idle');
export const activeDeploymentId = writable<number | null>(null);

// Toast notifications
let toastCounter = 0;
export const toasts = writable<ToastMessage[]>([]);

export function addToast(
  type: ToastMessage['type'],
  message: string,
  duration = 4000
) {
  const id = ++toastCounter;
  toasts.update((t) => [...t, { id, type, message }]);
  setTimeout(() => {
    toasts.update((t) => t.filter((m) => m.id !== id));
  }, duration);
}

// A live attempt exists during the build, before SQLite allocates its run ID.
export const activeDeployment = writable<DeploymentDTO | null>(null);

export function beginDeploymentAttempt(
  projectName: string,
  connectionName: string
) {
  const project = get(projects).find((p) => p.name === projectName);
  const connection = get(connections).find((c) => c.name === connectionName);
  selectedProjectName.set(projectName);
  selectedConnectionName.set(connectionName);
  deployLogs.set([]);
  deployPhase.set('build');
  activeDeploymentId.set(0);
  activeDeployment.set({
    id: 0,
    projectId: project?.id ?? 0,
    projectName,
    projectType: project?.projectType,
    buildCommand: project?.buildCommand,
    outputRelPath: project?.outputRelPath,
    connectionId: connection?.id ?? 0,
    connectionName,
    connectionHost: connection?.host,
    remoteRoot: connection?.remoteRoot,
    status: 'running',
    currentPhase: 'build',
    backupDir: '',
    errorMessage: '',
    startedAt: new Date().toISOString(),
    finishedAt: '',
  });
  deployStatus.set('running');
}

export function updateDeploymentAttempt(update: Partial<DeploymentDTO>) {
  activeDeployment.update((attempt) =>
    attempt ? { ...attempt, ...update } : null
  );
  if (update.id !== undefined) activeDeploymentId.set(update.id);
}
