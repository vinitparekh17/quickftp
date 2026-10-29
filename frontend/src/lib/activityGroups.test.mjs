import assert from 'node:assert/strict';
import { groupTransfers, pageItems } from './activityGroups.ts';
const recovered = Array.from({ length: 3252 }, (_, index) => ({
  id: String(index),
  direction: 'Upload',
  connectionId: 1,
  connectionName: 'Test server',
  remotePath: `/site/file-${index}.dll`,
  localPath: `C:/publish/file-${index}.dll`,
  status: 'Interrupted',
  startedAt: '2026-10-25T08:00:00Z',
  error: 'Original failure detail',
}));
const groups = groupTransfers(recovered);
assert.equal(groups.length, 1);
assert.equal(groups[0].recovered, true);
assert.equal(groups[0].entries.length, 3252);
assert.equal(groups[0].attention, 3252);
assert.equal(pageItems(groups[0].entries, 0).length, 50);
assert.equal(pageItems(groups[0].entries, 65).length, 2);
assert.equal(
  pageItems(groups[0].entries, 65)[1].error,
  'Original failure detail'
);
const explicit = groupTransfers([
  { ...recovered[0], batchId: 'batch-a', batchName: 'API publish' },
  { ...recovered[1], batchId: 'batch-b', batchName: 'Other upload' },
  { ...recovered[2], direction: 'Deployment upload', deploymentId: 17 },
]);
assert.equal(explicit.length, 2);
assert.equal(explicit[0].recovered, false);
assert.equal(
  explicit.some((group) => group.entries.some((item) => item.deploymentId)),
  false
);
console.log(
  'Activity grouping tests passed: 3,252 records preserved, explicit batches separate, deployment rows excluded, pages bounded'
);

const sameBatchAcrossConnections = groupTransfers([
  { ...recovered[0], batchId: 'same-local-batch', connectionId: 1 },
  { ...recovered[1], batchId: 'same-local-batch', connectionId: 2 },
]);
assert.equal(sameBatchAcrossConnections.length, 2);
