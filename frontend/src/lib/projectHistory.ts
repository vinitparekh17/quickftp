import type { DeploymentDTO } from './types';

type HistoryAPI = {
  GetProjectDeployments?: (
    projectId: number,
    limit: number
  ) => Promise<DeploymentDTO[]>;
  GetDeploymentHistory: (limit: number) => Promise<DeploymentDTO[]>;
};

// Hot-reloaded frontend code can outlive the Go process that supplied its
// bindings. Use the old read-only history method when the scoped method is absent.
export async function loadProjectDeployments(
  api: HistoryAPI,
  projectId: number,
  limit = 200
): Promise<DeploymentDTO[]> {
  limit = Math.max(1, Math.min(500, limit));
  if (typeof api.GetProjectDeployments === 'function') {
    return (await api.GetProjectDeployments(projectId, limit)) || [];
  }
  let fetchLimit = Math.max(200, limit);
  for (;;) {
    const records = (await api.GetDeploymentHistory(fetchLimit)) || [];
    const matching = records.filter((run) => run.projectId === projectId);
    if (matching.length >= limit || records.length < fetchLimit)
      return matching.slice(0, limit);
    // Don't silently omit older project runs or pull an unbounded database into
    // an older frontend/backend session. A restarted backend uses the SQL filter.
    if (fetchLimit >= 10000)
      throw new Error(
        'Restart the updated desktop app to load this project’s older deployment records.'
      );
    fetchLimit = Math.min(10000, fetchLimit * 2);
  }
}
