import assert from 'node:assert/strict';
import {
  resolveDeploymentStatus,
  resolveStageState,
  isRollbackPossible,
  formatDuration,
  formatBytes,
  formatChecksum,
  DEPLOYMENT_STAGES,
} from './deploymentUtils.ts';

// 1. Status Resolution Tests
{
  // Completed run
  const completed = resolveDeploymentStatus(
    {
      id: 1,
      projectId: 1,
      connectionId: 1,
      status: 'completed',
      currentPhase: 'done',
      backupDir: '/backup',
      errorMessage: '',
      startedAt: '2026-10-24T10:00:00Z',
      finishedAt: '2026-10-24T10:02:00Z',
    },
    null,
    false
  );
  assert.equal(completed.key, 'completed');
  assert.equal(completed.label, 'Success');
  assert.equal(completed.badgeClass, 'badge-success');

  // Failed run
  const failed = resolveDeploymentStatus(
    {
      id: 2,
      projectId: 1,
      connectionId: 1,
      status: 'failed',
      currentPhase: 'upload',
      backupDir: '/backup',
      errorMessage: 'Network timeout',
      startedAt: '2026-10-24T10:00:00Z',
      finishedAt: '2026-10-24T10:01:00Z',
    },
    null,
    false
  );
  assert.equal(failed.key, 'failed');
  assert.equal(failed.label, 'Failed');

  // Cancelled run
  const cancelled = resolveDeploymentStatus(
    {
      id: 3,
      projectId: 1,
      connectionId: 1,
      status: 'cancelled',
      currentPhase: 'upload',
      backupDir: '/backup',
      errorMessage: 'context canceled',
      startedAt: '2026-10-24T10:00:00Z',
      finishedAt: '2026-10-24T10:01:00Z',
    },
    null,
    false
  );
  assert.equal(cancelled.key, 'cancelled');
  assert.equal(cancelled.label, 'Cancelled');

  // Interrupted run (status is 'running' in DB, but no active session is running it)
  const interrupted = resolveDeploymentStatus(
    {
      id: 4,
      projectId: 1,
      connectionId: 1,
      status: 'running',
      currentPhase: 'upload',
      backupDir: '/backup',
      errorMessage: '',
      startedAt: '2026-10-24T10:00:00Z',
      finishedAt: '',
    },
    null,
    false
  );
  assert.equal(interrupted.key, 'interrupted');
  assert.equal(interrupted.label, 'Interrupted');

  // Actively running run
  const running = resolveDeploymentStatus(
    {
      id: 5,
      projectId: 1,
      connectionId: 1,
      status: 'running',
      currentPhase: 'upload',
      backupDir: '/backup',
      errorMessage: '',
      startedAt: '2026-10-24T10:00:00Z',
      finishedAt: '',
    },
    5,
    true
  );
  assert.equal(running.key, 'running');
  assert.equal(running.label, 'Running');

  // Actively rolling back
  const rollingBack = resolveDeploymentStatus(
    {
      id: 6,
      projectId: 1,
      connectionId: 1,
      status: 'failed',
      currentPhase: 'upload',
      backupDir: '/backup',
      errorMessage: 'Failed to write file',
      startedAt: '2026-10-24T10:00:00Z',
      finishedAt: '',
    },
    null,
    false,
    6 // activeRollbackId
  );
  assert.equal(rollingBack.key, 'rolling_back');
  assert.equal(rollingBack.label, 'Rolling back');

  // Rolled back
  const rolledBack = resolveDeploymentStatus(
    {
      id: 7,
      projectId: 1,
      connectionId: 1,
      status: 'rolled_back',
      currentPhase: 'rollback',
      backupDir: '/backup',
      errorMessage: '',
      startedAt: '2026-10-24T10:00:00Z',
      finishedAt: '2026-10-24T10:05:00Z',
    },
    null,
    false
  );
  assert.equal(rolledBack.key, 'rolled_back');
  assert.equal(rolledBack.label, 'Rolled back');

  // Rollback failed
  const failedRollbackIds = new Set([8]);
  const rollbackFailed = resolveDeploymentStatus(
    {
      id: 8,
      projectId: 1,
      connectionId: 1,
      status: 'failed',
      currentPhase: 'upload',
      backupDir: '/backup',
      errorMessage: 'Original error',
      startedAt: '2026-10-24T10:00:00Z',
      finishedAt: '2026-10-24T10:05:00Z',
    },
    null,
    false,
    null,
    failedRollbackIds
  );
  assert.equal(rollbackFailed.key, 'rollback_failed');
  assert.equal(rollbackFailed.label, 'Rollback failed');
}

// 2. Formatting tests
{
  assert.equal(formatDuration('2026-10-24T10:00:00Z', '2026-10-24T10:00:45Z'), '45s');
  assert.equal(formatDuration('2026-10-24T10:00:00Z', '2026-10-24T10:02:15Z'), '2m 15s');

  assert.equal(formatBytes(0), '0 B');
  assert.equal(formatBytes(512), '512 B');
  assert.equal(formatBytes(1536), '1.5 KB');
  assert.equal(formatBytes(1048576 * 2.5), '2.5 MB');

  assert.equal(
    formatChecksum('3a7bd3e2360a3d29eea436fcfb7e44c735d117c42d1c1835420b6b9942dd4f1b'),
    '3a7bd3e236...dd4f1b'
  );
}

// 3. Stage state tests
{
  // When completed, every stage is completed
  assert.equal(resolveStageState('build', 'complete', 'completed'), 'completed');
  assert.equal(resolveStageState('upload', 'complete', 'completed'), 'completed');

  // When running at 'upload', previous stages completed, current running, later pending
  assert.equal(resolveStageState('build', 'upload', 'running'), 'completed');
  assert.equal(resolveStageState('upload', 'upload', 'running'), 'running');
  assert.equal(resolveStageState('delete', 'upload', 'running'), 'pending');

  // IIS offline skipped if not enabled
  assert.equal(resolveStageState('iis_offline', 'complete', 'completed', false), 'skipped');
}

// 4. Rollback Eligibility Tests
{
  const run1 = {
    id: 1,
    projectId: 1,
    connectionId: 1,
    status: 'completed',
    currentPhase: 'done',
    backupDir: '/backup/1',
    errorMessage: '',
    startedAt: '2026-10-24T10:00:00Z',
    finishedAt: '2026-10-24T10:01:00Z',
  };
  const run2FailedOld = {
    id: 2,
    projectId: 1,
    connectionId: 1,
    status: 'failed',
    currentPhase: 'upload',
    backupDir: '/backup/2',
    errorMessage: 'Connection lost',
    startedAt: '2026-10-24T10:05:00Z',
    finishedAt: '2026-10-24T10:06:00Z',
  };
  const run3 = {
    id: 3,
    projectId: 1,
    connectionId: 1,
    status: 'completed',
    currentPhase: 'done',
    backupDir: '/backup/3',
    errorMessage: '',
    startedAt: '2026-10-24T10:10:00Z',
    finishedAt: '2026-10-24T10:11:00Z',
  };
  const run4 = {
    id: 4,
    projectId: 1,
    connectionId: 1,
    status: 'completed',
    currentPhase: 'done',
    backupDir: '/backup/4',
    errorMessage: '',
    startedAt: '2026-10-24T10:15:00Z',
    finishedAt: '2026-10-24T10:16:00Z',
  };
  const run5LatestFailed = {
    id: 5,
    projectId: 1,
    connectionId: 1,
    status: 'failed',
    currentPhase: 'upload',
    backupDir: '/backup/5',
    errorMessage: 'Checksum mismatch',
    startedAt: '2026-10-24T10:20:00Z',
    finishedAt: '2026-10-24T10:21:00Z',
  };

  // History list newest-first: [5, 4, 3, 2, 1]
  const historyWithLatestFailed = [
    run5LatestFailed,
    run4,
    run3,
    run2FailedOld,
    run1,
  ];

  // Test: Most recent deployment (#5) failed -> restore IS possible!
  assert.equal(
    isRollbackPossible(run5LatestFailed, historyWithLatestFailed),
    true,
    'Latest failed deployment should be eligible for rollback'
  );

  // Test: Failed 3 deployments ago (#2) -> do NOT show restore button!
  assert.equal(
    isRollbackPossible(run2FailedOld, historyWithLatestFailed),
    false,
    'Failed deployment 3 runs ago must NOT show restore button'
  );

  // Test: Completed deployments must never show restore button
  assert.equal(isRollbackPossible(run4, historyWithLatestFailed), false);
  assert.equal(isRollbackPossible(run3, historyWithLatestFailed), false);

  // Test: If the most recent deployment succeeded (#6 deployed after #5):
  const run6Success = {
    id: 6,
    projectId: 1,
    connectionId: 1,
    status: 'completed',
    currentPhase: 'done',
    backupDir: '/backup/6',
    errorMessage: '',
    startedAt: '2026-10-24T10:25:00Z',
    finishedAt: '2026-10-24T10:26:00Z',
  };
  const historyAfterRun6 = [run6Success, ...historyWithLatestFailed];

  // Run 5 is no longer the most recent deployment on this connection:
  assert.equal(
    isRollbackPossible(run5LatestFailed, historyAfterRun6),
    false,
    'If a subsequent deployment has already run, older failed run cannot be rolled back'
  );

  // Test: Consecutive failures: [8 (failed), 7 (failed)]
  const run7 = {
    id: 7,
    projectId: 1,
    connectionId: 1,
    status: 'failed',
    currentPhase: 'upload',
    backupDir: '/backup/7',
    errorMessage: 'Error 7',
    startedAt: '2026-10-24T10:30:00Z',
    finishedAt: '2026-10-24T10:31:00Z',
  };
  const run8 = {
    id: 8,
    projectId: 1,
    connectionId: 1,
    status: 'failed',
    currentPhase: 'upload',
    backupDir: '/backup/8',
    errorMessage: 'Error 8',
    startedAt: '2026-10-24T10:35:00Z',
    finishedAt: '2026-10-24T10:36:00Z',
  };
  const consecutiveFailures = [run8, run7, run6Success];

  // Most recent failed (#8) can be restored:
  assert.equal(isRollbackPossible(run8, consecutiveFailures), true);
  // Previous failed (#7, not latest failed deployment) CANNOT be restored:
  assert.equal(isRollbackPossible(run7, consecutiveFailures), false);
}

console.log('All deployment utility tests passed successfully.');

