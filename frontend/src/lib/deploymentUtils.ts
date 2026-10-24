import type { DeploymentDTO, DeploymentExecutionStatus } from './types';
import type { IconName } from './icons';

export interface PipelineStage {
  key: string;
  label: string;
  description: string;
  isDestructive?: boolean;
}

export const DEPLOYMENT_STAGES: PipelineStage[] = [
  {
    key: 'build',
    label: 'Build & Package',
    description: 'Compile source and assemble deployable artifacts',
  },
  {
    key: 'planning',
    label: 'Plan & Checksum',
    description: 'Calculate local SHA-256 digests and validate safe relative paths',
  },
  {
    key: 'discover',
    label: 'Remote Discovery',
    description: 'Inspect existing remote directory tree before making changes',
  },
  {
    key: 'backup',
    label: 'Remote Snapshot Backup',
    description: 'Download existing remote files to local deployment backup directory',
  },
  {
    key: 'verify_backup',
    label: 'Verify Backup Integrity',
    description: 'Confirm file sizes and SHA-256 checksums of local backup',
  },
  {
    key: 'iis_offline',
    label: 'IIS Maintenance Mode',
    description: 'Quiesce ASP.NET Core host using app_offline.htm to unlock locked DLLs',
  },
  {
    key: 'upload',
    label: 'Upload & Stage',
    description: 'Upload files to temporary staging names (.quickftp-<id>.tmp)',
  },
  {
    key: 'verify_upload',
    label: 'Verify & Promote',
    description: 'Verify remote SHA-256 digests against local hashes and atomically rename',
  },
  {
    key: 'delete',
    label: 'Prune Stale Files',
    description: 'Remove remote files not present in the new release (verified backup required)',
    isDestructive: true,
  },
  {
    key: 'complete',
    label: 'Finalize Deployment',
    description: 'Verify final remote state and commit durable journal record',
  },
];

export interface StatusMeta {
  key: DeploymentExecutionStatus;
  label: string;
  badgeClass: string;
  icon: IconName;
  spin: boolean;
  desc: string;
}

/**
 * Resolves the explicit status of a deployment run based on backend state.
 */
export function resolveDeploymentStatus(
  d: DeploymentDTO,
  activeDeploymentId: number | null,
  isGlobalDeployRunning: boolean,
  activeRollbackId: number | null = null,
  failedRollbackIds: Set<number> = new Set()
): StatusMeta {
  // 1. Actively rolling back right now
  if (activeRollbackId === d.id) {
    return {
      key: 'rolling_back',
      label: 'Rolling back',
      badgeClass: 'badge-warning',
      icon: 'rollback',
      spin: true,
      desc: 'Restoring remote host to previous verified backup snapshot',
    };
  }

  // 2. Rollback was attempted but failed
  if (failedRollbackIds.has(d.id)) {
    return {
      key: 'rollback_failed',
      label: 'Rollback failed',
      badgeClass: 'badge-danger',
      icon: 'error',
      spin: false,
      desc: 'Rollback operation encountered an error while restoring files',
    };
  }

  // 3. Successfully rolled back
  if (d.status === 'rolled_back') {
    return {
      key: 'rolled_back',
      label: 'Rolled back',
      badgeClass: 'badge-gray',
      icon: 'rollback',
      spin: false,
      desc: 'Remote host was restored from verified backup snapshot',
    };
  }

  // 4. Cancelled mid-deployment
  if (
    d.cancelled === true ||
    d.status === 'cancelled' ||
    (d.errorMessage &&
      (d.errorMessage.includes('context canceled') ||
        d.errorMessage.toLowerCase().includes('cancelled') ||
        d.errorMessage.toLowerCase().includes('canceled')))
  ) {
    return {
      key: 'cancelled',
      label: 'Cancelled',
      badgeClass: 'badge-warning',
      icon: 'warning',
      spin: false,
      desc: 'Deployment was stopped before completing all transfer stages',
    };
  }

  // 5. Interrupted (persisted as 'running' in SQLite from an earlier crashed session)
  const isCurrentlyActiveInSession =
    isGlobalDeployRunning && activeDeploymentId === d.id;

  if (d.status === 'running' && !isCurrentlyActiveInSession) {
    return {
      key: 'interrupted',
      label: 'Interrupted',
      badgeClass: 'badge-warning',
      icon: 'warning',
      spin: false,
      desc: 'Run was interrupted before completion. Rollback is available.',
    };
  }

  // 6. Actively running in current session
  if (d.status === 'running' && isCurrentlyActiveInSession) {
    return {
      key: 'running',
      label: 'Running',
      badgeClass: 'badge-info',
      icon: 'loader',
      spin: true,
      desc: 'Deployment pipeline is currently executing',
    };
  }

  // 7. Succeeded (completed)
  if (d.status === 'completed') {
    return {
      key: 'completed',
      label: 'Success',
      badgeClass: 'badge-success',
      icon: 'success',
      spin: false,
      desc: 'All artifacts uploaded and verified successfully',
    };
  }

  // 8. Failed
  if (d.status === 'failed') {
    return {
      key: 'failed',
      label: 'Failed',
      badgeClass: 'badge-danger',
      icon: 'error',
      spin: false,
      desc: d.errorMessage || 'Deployment failed during execution',
    };
  }

  // Default fallback
  return {
    key: 'queued',
    label: d.status || 'Queued',
    badgeClass: 'badge-gray',
    icon: 'clock',
    spin: false,
    desc: 'Deployment is queued for execution',
  };
}

/**
 * Resolves the state of an individual pipeline stage for a deployment run.
 */
export function resolveStageState(
  stageKey: string,
  currentPhase: string,
  overallStatus: DeploymentExecutionStatus,
  hasOfflinePage = false
): 'pending' | 'running' | 'completed' | 'failed' | 'cancelled' | 'skipped' {
  // If stage is IIS and offline page was not configured, it was skipped
  if (stageKey === 'iis_offline' && !hasOfflinePage) {
    return 'skipped';
  }

  const stageKeys = DEPLOYMENT_STAGES.map((s) => s.key);
  const currentIdx = stageKeys.indexOf(currentPhase);
  const targetIdx = stageKeys.indexOf(stageKey);

  if (overallStatus === 'completed') {
    return 'completed';
  }

  if (overallStatus === 'failed' && stageKey === currentPhase) {
    return 'failed';
  }

  if (overallStatus === 'cancelled' && stageKey === currentPhase) {
    return 'cancelled';
  }

  if (overallStatus === 'running') {
    if (stageKey === currentPhase) return 'running';
    if (currentIdx >= 0 && targetIdx < currentIdx) return 'completed';
    return 'pending';
  }

  if (overallStatus === 'interrupted' || overallStatus === 'failed' || overallStatus === 'cancelled') {
    if (currentIdx >= 0 && targetIdx < currentIdx) return 'completed';
    if (stageKey === currentPhase) {
      return overallStatus === 'cancelled' ? 'cancelled' : 'failed';
    }
    return 'pending';
  }

  if (overallStatus === 'rolled_back' || overallStatus === 'rolling_back') {
    if (currentIdx >= 0 && targetIdx < currentIdx) return 'completed';
    if (stageKey === currentPhase) return 'failed';
    return 'pending';
  }

  return 'pending';
}

/**
 * Formats an ISO date string into a clean, localized string.
 */
export function formatDateTime(iso: string): string {
  if (!iso) return '—';
  try {
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toLocaleString([], {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  } catch {
    return iso;
  }
}

/**
 * Calculates human-readable duration between start and finish.
 */
export function formatDuration(start: string, finish?: string): string {
  if (!start) return '—';
  try {
    const s = new Date(start).getTime();
    const f = finish ? new Date(finish).getTime() : Date.now();
    if (isNaN(s) || (finish && isNaN(f))) return '—';
    const totalSec = Math.max(0, Math.round((f - s) / 1000));
    if (totalSec < 60) return `${totalSec}s`;
    const min = Math.floor(totalSec / 60);
    const sec = totalSec % 60;
    return `${min}m ${sec}s`;
  } catch {
    return '—';
  }
}

/**
 * Formats byte size into human readable string.
 */
export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  if (!bytes || isNaN(bytes)) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const val = bytes / Math.pow(1024, i);
  return `${val.toFixed(i === 0 ? 0 : 1)} ${units[i] || 'B'}`;
}

/**
 * Truncates an SHA-256 digest for readable compact display.
 */
export function formatChecksum(sha: string): string {
  if (!sha) return '—';
  if (sha.length <= 16) return sha;
  return `${sha.substring(0, 10)}...${sha.substring(sha.length - 6)}`;
}

/**
 * Determines whether rollback / restore is possible for a deployment run.
 * Rules:
 * 1. Only the most recent deployment for a target connection can be rolled back.
 * 2. If a failed deployment is older (e.g. 3 deployments ago or not the latest deployment),
 *    the restore button MUST NOT be shown.
 * 3. The deployment must be in a failed, cancelled, or interrupted state.
 * 4. A verified backup directory must exist.
 * 5. It must not have already been rolled back.
 */
export function isRollbackPossible(
  d: DeploymentDTO,
  allDeployments: DeploymentDTO[]
): boolean {
  if (!d || !d.backupDir) return false;
  if (d.status === 'completed' || d.status === 'rolled_back') return false;

  const isFailedOrCancelled =
    d.status === 'failed' ||
    d.status === 'cancelled' ||
    d.cancelled === true ||
    (d.status === 'running' && Boolean(d.errorMessage));

  if (!isFailedOrCancelled) return false;

  // Cannot rollback if any newer deployment exists for the same target connection
  const hasNewerOnConnection = allDeployments.some(
    (other) => other.id > d.id && other.connectionId === d.connectionId
  );
  if (hasNewerOnConnection) return false;

  // Cannot rollback if this is not the latest failed deployment across the board
  const latestFailedRun = allDeployments.find(
    (other) =>
      other.status === 'failed' ||
      other.status === 'cancelled' ||
      other.cancelled === true ||
      (other.status === 'running' && Boolean(other.errorMessage))
  );
  if (!latestFailedRun || latestFailedRun.id !== d.id) {
    return false;
  }

  return true;
}

