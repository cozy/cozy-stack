import { deepEqual, equal, throws } from 'node:assert/strict'
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, it } from 'node:test'

import {
  createSharedDriveActivityCapacityScenario,
  getSharedDriveMetrics,
  type SharedDriveCampaignConfig,
} from './shared-drive-activity.ts'

describe('SharedDriveActivityCapacityScenario', (): void => {
  function makeTestDir(): string {
    return mkdtempSync(join(tmpdir(), 'shared-drive-scenario-test-'))
  }

  it('parses composite P95 and success rates from summary JSON', (): void => {
    const testDir = makeTestDir()
    try {
      const summaryPath = join(testDir, 'summary.json')
      writeFileSync(
        summaryPath,
        JSON.stringify({
          metrics: {
            http_req_duration: {
              'p(95)': 120.0,
            },
            shared_drive_changes_duration_ms: {
              'p(95)': 185.5,
            },
            shared_drive_upload_success_rate: {
              value: 0.99,
            },
            shared_drive_ws_handshake_rate: {
              value: 1.0,
            },
            shared_drive_changes_success_rate: {
              value: 0.97,
            },
            iterations: {
              count: 120,
            },
            cleanup_success: {
              value: 1.0,
            },
            dropped_iterations: {
              count: 0,
            },
          },
        }),
      )

      const metrics = getSharedDriveMetrics(summaryPath)
      // Composite P95 should be max(120.0, 185.5) = 185.5
      equal(metrics?.p95Ms, 185.5)
      // Success rate should be min(0.99, 1.0, 0.97) = 0.97
      equal(metrics?.successRate, 0.97)
      equal(metrics?.completedIterations, 120)
      equal(metrics?.cleanupRate, 1.0)
      equal(metrics?.droppedIterations, 0)
    } finally {
      rmSync(testDir, { recursive: true, force: true })
    }
  })

  it('validates preflight requirements', (): void => {
    const scenario = createSharedDriveActivityCapacityScenario(
      '/tmp/harness',
      '/tmp/results',
    )

    const invalidConfig: SharedDriveCampaignConfig = {
      baselineIterations: 5,
      campaignId: 'test-sd',
      confirmationIterations: 10,
      configuredP95LimitMs: null,
      fileSize: '100K',
      latencyMultiplier: 2,
      maxLevel: 8,
      minimumSuccessRate: 0.95,
      sharedDriveId: '',
      sharedDriveRootId: '',
      targetUrl: 'http://localhost',
    }

    throws((): void => {
      scenario.validatePreflight?.(invalidConfig)
    }, /SHARED_DRIVE_ID and SHARED_DRIVE_ROOT_ID must be configured/)
  })

  it('builds expected report metadata with default ratios', (): void => {
    const scenario = createSharedDriveActivityCapacityScenario(
      '/tmp/harness',
      '/tmp/results',
    )

    const config: SharedDriveCampaignConfig = {
      baselineIterations: 5,
      campaignId: 'test-sd',
      confirmationIterations: 10,
      configuredP95LimitMs: null,
      fileSize: '100K',
      latencyMultiplier: 2,
      maxLevel: 8,
      minimumSuccessRate: 0.95,
      sharedDriveId: 'drive-123',
      sharedDriveRootId: 'root-456',
      targetUrl: 'http://localhost',
    }

    const metadata = scenario.buildReportMetadata?.(config)
    deepEqual(metadata, {
      shared_drive_id: 'drive-123',
      shared_drive_root_id: 'root-456',
      file_size: '100K',
      writer_ratio: 1,
      ws_ratio: 3,
      changes_ratio: 2,
    })
  })
})
