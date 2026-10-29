import assert from 'node:assert/strict';
import { loadProjectDeployments } from './projectHistory.ts';
const records = Array.from({ length: 250 }, (_, index) => ({
  id: 250 - index,
  projectId: index < 230 ? 2 : 1,
}));
const requests = [];
const olderAPI = {
  GetDeploymentHistory: async (limit) => {
    requests.push(limit);
    return records.slice(0, limit);
  },
};
const recovered = await loadProjectDeployments(olderAPI, 1);
assert.equal(recovered.length, 20);
assert.equal(
  recovered.every((run) => run.projectId === 1),
  true
);
assert.deepEqual(requests, [200, 400]);
let nativeCalls = 0;
const nativeAPI = {
  GetProjectDeployments: async (id, limit) => {
    nativeCalls++;
    assert.equal(id, 1);
    return records.filter((run) => run.projectId === id).slice(0, limit);
  },
  GetDeploymentHistory: async () => {
    throw Error('should not use legacy fallback');
  },
};
assert.equal((await loadProjectDeployments(nativeAPI, 1, 5)).length, 5);
assert.equal(nativeCalls, 1);
await assert.rejects(
  () =>
    loadProjectDeployments(
      {
        ...nativeAPI,
        GetProjectDeployments: async () => {
          throw Error('database unavailable');
        },
      },
      1
    ),
  /database unavailable/
);
assert.deepEqual(
  await loadProjectDeployments({ GetDeploymentHistory: async () => null }, 1),
  []
);
console.log(
  'Project history compatibility tests passed: absent binding, older runs, native API and real backend errors'
);
