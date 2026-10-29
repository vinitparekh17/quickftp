import type { DeploymentDTO } from './types';

// Zero selects this session's live attempt, which may not have a journal ID yet.
export function selectDeployment(
  selectedRunId: number | null,
  history: DeploymentDTO[],
  live: DeploymentDTO | null
): DeploymentDTO | null {
  if (selectedRunId !== 0)
    return history.find((run) => run.id === selectedRunId) ?? null;
  if (!live) return null;
  const recorded = history.find((run) => live.id > 0 && run.id === live.id);
  if (recorded && live.status !== 'running' && recorded.status !== 'running')
    return recorded;
  return {
    ...recorded,
    ...live,
    backupDir: recorded?.backupDir || live.backupDir,
  };
}
