import { deepEqual, equal } from 'node:assert/strict'
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, it } from 'node:test'

import {
  createUploadCapacityScenario,
  getPointMetrics,
  type UploadCampaignConfig,
} from './upload.ts'

describe('UploadCapacityScenario', (): void => {
  function makeTestDir(): string {
    return mkdtempSync(join(tmpdir(), 'upload-scenario-test-'))
  }

  it('parses valid point metrics from k6 summary JSON', (): void => {
    const testDir = makeTestDir()
    try {
      const summaryPath = join(testDir, 'summary.json')
      writeFileSync(
        summaryPath,
        JSON.stringify({
          metrics: {
            'http_req_duration{operation:file-upload}': {
              'p(95)': 142.5,
            },
            'upload_success{operation:file-upload}': {
              value: 0.995,
            },
            iterations: {
              count: 50,
            },
            cleanup_success: {
              value: 1,
            },
            dropped_iterations: {
              count: 0,
            },
          },
        }),
      )

      const metrics = getPointMetrics(summaryPath)
      equal(metrics?.p95Ms, 142.5)
      equal(metrics?.successRate, 0.995)
      equal(metrics?.completedIterations, 50)
      equal(metrics?.cleanupRate, 1)
      equal(metrics?.droppedIterations, 0)
    } finally {
      rmSync(testDir, { recursive: true, force: true })
    }
  })

  it('returns null on missing or malformed summary JSON', (): void => {
    const testDir = makeTestDir()
    try {
      const summaryPath = join(testDir, 'bad.json')
      writeFileSync(summaryPath, 'not json')
      equal(getPointMetrics(summaryPath), null)

      writeFileSync(summaryPath, JSON.stringify({ metrics: {} }))
      equal(getPointMetrics(summaryPath), null)
    } finally {
      rmSync(testDir, { recursive: true, force: true })
    }
  })

  it('builds expected report metadata', (): void => {
    const scenario = createUploadCapacityScenario('/tmp/harness', '/tmp/results')
    const config: UploadCampaignConfig = {
      baselineIterations: 10,
      campaignId: 'test-upload',
      confirmationIterations: 20,
      configuredP95LimitMs: null,
      consistencyChunkSizeBytes: 1024,
      fileSize: '1M',
      fileSizeBytes: 1048576,
      forceCapacityRun: false,
      latencyMultiplier: 2,
      maxLevel: 16,
      maxVus: 16,
      minimumSuccessRate: 0.99,
      targetUrl: 'http://localhost',
      uploadTarget: { kind: 'personal', rootDirectoryId: 'io.cozy.files.root-dir' },
    }

    const metadata = scenario.buildReportMetadata?.(config)
    deepEqual(metadata, {
      upload_target: 'personal',
      file_size: '1M',
      file_size_bytes: 1048576,
      configured_max_vus: 16,
    })
  })
})
