import { writable } from 'svelte/store';
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
export const activityPanelOpen = writable<boolean>(
  localStorage.getItem('quickftp.activity.open') === 'true'
);
export const activityPanelTab = writable<'Transfers' | 'Logs'>('Transfers');
export const activityPanelHeight = writable<number>(
  Number(localStorage.getItem('quickftp.activity.height')) || 300
);
export const acknowledgedFailures = writable<string[]>(
  JSON.parse(localStorage.getItem('quickftp.activity.acknowledged') || '[]')
);
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
