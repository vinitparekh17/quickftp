import assert from 'node:assert/strict';
import { get } from 'svelte/store';
import { selectDeployment } from './deploymentSelection.ts';

globalThis.localStorage = { getItem: () => null };
const stores = await import('./stores.ts');
const old = { id: 15, status: 'failed', errorMessage: 'old 426 failure' };
stores.activeDeploymentId.set(15);
stores.deployLogs.set([{ stream: 'stderr', line: 'old 426 failure' }]);
stores.deployStatus.set('failed');
stores.beginDeploymentAttempt('api', 'test');
const live = get(stores.activeDeployment);
assert.equal(get(stores.activeDeploymentId), 0);
assert.deepEqual(get(stores.deployLogs), []);
assert.equal(get(stores.deployPhase), 'build');
assert.equal(get(stores.deployStatus), 'running');
assert.equal(selectDeployment(0, [old], live).id, 0);
assert.equal(selectDeployment(0, [old], live).errorMessage, '');
assert.equal(selectDeployment(15, [old], live), old);
assert.equal(selectDeployment(null, [old], live), null);

stores.updateDeploymentAttempt({ id: 16, currentPhase: 'upload' });
assert.equal(get(stores.activeDeploymentId), 16);
assert.equal(selectDeployment(0, [old], get(stores.activeDeployment)).id, 16);
stores.updateDeploymentAttempt({
  status: 'failed',
  errorMessage: 'new failure',
});
assert.equal(
  selectDeployment(0, [old], get(stores.activeDeployment)).errorMessage,
  'new failure'
);
const rolledBack = { id: 16, status: 'rolled_back', backupDir: '/backup' };
assert.equal(
  selectDeployment(0, [old, rolledBack], get(stores.activeDeployment)),
  rolledBack
);
console.log('Deployment selection regression tests passed');
