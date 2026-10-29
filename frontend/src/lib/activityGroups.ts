import type { TransferDTO } from './types';

export const ACTIVITY_PAGE_SIZE = 50;
export const isActiveTransfer = (status: string) =>
  ['Queued', 'Running', 'Waiting'].includes(status);
export const transferNeedsAttention = (status: string) =>
  ['Waiting', 'Failed', 'Interrupted'].includes(status);

export interface TransferGroup {
  id: string;
  label: string;
  connectionId: number;
  connectionName: string;
  startedAt: string;
  recovered: boolean;
  entries: TransferDTO[];
  active: number;
  attention: number;
}

// Older recovery rows have no deployment or batch identity. Group them honestly
// by connection/direction/start date; never infer a deployment from proximity.
export function groupTransfers(items: TransferDTO[]): TransferGroup[] {
  const groups = new Map<string, TransferGroup>();
  for (const item of items) {
    if (item.deploymentId || item.direction === 'Deployment upload') continue;
    const recovered = !item.batchId;
    const day = item.startedAt?.slice(0, 10) || 'unknown date';
    const id = item.batchId
      ? `connection:${item.connectionId}:batch:${item.batchId}`
      : `recovered:${item.connectionId}:${item.direction}:${day}`;
    let group = groups.get(id);
    if (!group) {
      group = {
        id,
        label:
          item.batchName ||
          `${item.direction === 'Upload' ? 'Uploads' : 'Downloads'} · ${day}`,
        connectionId: item.connectionId,
        connectionName: item.connectionName,
        startedAt: item.startedAt || '',
        recovered,
        entries: [],
        active: 0,
        attention: 0,
      };
      groups.set(id, group);
    }
    group.entries.push(item);
    if (isActiveTransfer(item.status)) group.active++;
    if (transferNeedsAttention(item.status)) group.attention++;
    if (item.startedAt > group.startedAt) group.startedAt = item.startedAt;
  }
  return [...groups.values()].sort(
    (a, b) =>
      Number(b.active > 0) - Number(a.active > 0) ||
      b.startedAt.localeCompare(a.startedAt)
  );
}

export function pageItems<T>(
  items: T[],
  page: number,
  size = ACTIVITY_PAGE_SIZE
): T[] {
  return items.slice(Math.max(0, page) * size, (Math.max(0, page) + 1) * size);
}
