import { writable } from 'svelte/store';
import type { Page, ConnectionDTO, ProjectDTO, DeploymentDTO, ToastMessage, DeployLog } from './types';

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

// Deploy state
export const deployLogs = writable<DeployLog[]>([]);
export const deployPhase = writable<string>('');
export const deployStatus = writable<'idle' | 'running' | 'completed' | 'failed' | 'cancelled'>('idle');
export const activeDeploymentId = writable<number | null>(null);

// Toast notifications
let toastCounter = 0;
export const toasts = writable<ToastMessage[]>([]);

export function addToast(type: ToastMessage['type'], message: string, duration = 4000) {
  const id = ++toastCounter;
  toasts.update(t => [...t, { id, type, message }]);
  setTimeout(() => {
    toasts.update(t => t.filter(m => m.id !== id));
  }, duration);
}
