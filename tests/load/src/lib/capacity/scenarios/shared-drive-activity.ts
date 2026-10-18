import { spawnSync } from 'node:child_process'
import { existsSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'

import type { UploadDataSize } from '../../../fixtures/upload-sizes.ts'
import type { BaseCapacityConfig } from '../engine.ts'
import { getMetricNumber, isJsonRecord } from '../metrics.ts'
import type {
  CapacityPhase,
  CapacityScenario,
  PointRunResult,
} from '../types.ts'

export type SharedDriveCampaignConfig = BaseCapacityConfig & {
  fileSize: UploadDataSize
  sharedDriveId: string
  sharedDriveRootId: string
  cozyAccessToken?: string
  duration?: string
  writerRatio?: number
  wsRatio?: number
  changesRatio?: number
}

type SharedDriveMetrics = {
  cleanupRate: number
  completedIterations: number
  droppedIterations: number
  p95Ms: number
  successRate: number
}

export function getSharedDriveMetrics(
  summaryPath: string,
): SharedDriveMetrics | null {
  try {
    const parsedSummary: unknown = JSON.parse(readFileSync(summaryPath, 'utf8'))

    if (!isJsonRecord(parsedSummary)) {
      return null
    }

    const httpP95 = getMetricNumber(
      parsedSummary,
      ['http_req_duration'],
      'p(95)',
    )
    const changesP95 = getMetricNumber(
      parsedSummary,
      ['shared_drive_changes_duration_ms'],
      'p(95)',
    )

    const p95Ms = Math.max(httpP95 ?? 0, changesP95 ?? 0)

    const uploadSuccess = getMetricNumber(
      parsedSummary,
      ['shared_drive_upload_success_rate'],
      'value',
    )
    const wsHandshake = getMetricNumber(
      parsedSummary,
      ['shared_drive_ws_handshake_rate'],
      'value',
    )
    const changesSuccess = getMetricNumber(
      parsedSummary,
      ['shared_drive_changes_success_rate'],
      'value',
    )

    const successRate = Math.min(
      uploadSuccess ?? 1,
      wsHandshake ?? 1,
      changesSuccess ?? 1,
    )

    const completedIterations =
      getMetricNumber(parsedSummary, ['iterations'], 'count') ?? 0
    const cleanupRate =
      getMetricNumber(parsedSummary, ['cleanup_success'], 'value') ?? 1
    const droppedIterations =
      getMetricNumber(parsedSummary, ['dropped_iterations'], 'count') ?? 0

    return {
      cleanupRate,
      completedIterations,
      droppedIterations,
      p95Ms,
      successRate,
    }
  } catch (_error: unknown) {
    return null
  }
}

export function createSharedDriveActivityCapacityScenario(
  harnessDirectory: string,
  resultsDirectory: string,
): CapacityScenario<SharedDriveCampaignConfig> {
  return {
    name: 'shared-drive-activity',
    unit: 'scale units (1W:3WS:2C)',
    validatePreflight(config: SharedDriveCampaignConfig): void {
      if (!config.sharedDriveId || !config.sharedDriveRootId) {
        throw new Error(
          'SHARED_DRIVE_ID and SHARED_DRIVE_ROOT_ID must be configured for shared-drive-activity capacity search',
        )
      }
    },
    buildReportMetadata(
      config: SharedDriveCampaignConfig,
    ): Record<string, unknown> {
      return {
        shared_drive_id: config.sharedDriveId,
        shared_drive_root_id: config.sharedDriveRootId,
        file_size: config.fileSize,
        writer_ratio: config.writerRatio ?? 1,
        ws_ratio: config.wsRatio ?? 3,
        changes_ratio: config.changesRatio ?? 2,
      }
    },
    runPoint(
      phase: CapacityPhase,
      level: number,
      _iterationsPerVu: number,
      p95LimitMs: number | null,
      config: SharedDriveCampaignConfig,
      _attemptNumber: number,
      executionId: string,
    ): PointRunResult {
      const summaryName = `${executionId}-shared-drive-summary.json`
      const summaryPath = join(resultsDirectory, summaryName)

      const writerRatio = config.writerRatio ?? 1
      const wsRatio = config.wsRatio ?? 3
      const changesRatio = config.changesRatio ?? 2

      const writerVus = Math.max(1, level * writerRatio)
      const wsReaderVus = Math.max(1, level * wsRatio)
      const changesReaderVus = Math.max(1, level * changesRatio)

      const duration = config.duration ?? '20s'

      process.stdout.write(
        `\nRunning ${phase} point: level ${level} (${writerVus} writers, ${wsReaderVus} ws_readers, ${changesReaderVus} changes_readers) for ${duration}\n`,
      )

      const makeArguments = [
        '-C',
        harnessDirectory,
        'cozy-shared-drive-load',
        `COZY_BASE_URL=${config.targetUrl}`,
        `COZY_ACCESS_TOKEN=${config.cozyAccessToken ?? process.env.COZY_ACCESS_TOKEN ?? ''}`,
        `SHARED_DRIVE_ID=${config.sharedDriveId}`,
        `SHARED_DRIVE_ROOT_ID=${config.sharedDriveRootId}`,
        `FILE_SIZE=${config.fileSize}`,
        `DURATION=${duration}`,
        `WRITER_VUS=${writerVus}`,
        `WS_READER_VUS=${wsReaderVus}`,
        `CHANGES_READER_VUS=${changesReaderVus}`,
        `EXECUTION_ID=${executionId}`,
      ]

      const makeResult = spawnSync('make', makeArguments, {
        stdio: 'inherit',
      })
      const runnerExitCode = makeResult.status ?? 2

      if (!existsSync(summaryPath) || statSync(summaryPath).size === 0) {
        return {
          result: 'infrastructure_failure',
          reason: 'k6 produced no summary',
          runnerExitCode,
          summaryPath: `data/results/${summaryName}`,
          p95Ms: null,
          successRate: null,
          droppedIterations: null,
          completedIterations: null,
          cleanupRate: null,
        }
      }

      const metrics = getSharedDriveMetrics(summaryPath)

      if (metrics === null) {
        return {
          result: 'infrastructure_failure',
          reason: 'summary is missing required metrics',
          runnerExitCode,
          summaryPath: `data/results/${summaryName}`,
          p95Ms: null,
          successRate: null,
          droppedIterations: null,
          completedIterations: null,
          cleanupRate: null,
        }
      }

      if (metrics.cleanupRate !== 1) {
        return {
          result: 'infrastructure_failure',
          reason: 'test-directory cleanup failed',
          runnerExitCode,
          summaryPath: `data/results/${summaryName}`,
          p95Ms: metrics.p95Ms,
          successRate: metrics.successRate,
          droppedIterations: metrics.droppedIterations,
          completedIterations: metrics.completedIterations,
          cleanupRate: metrics.cleanupRate,
        }
      }

      let failureReason = ''
      if (metrics.successRate < config.minimumSuccessRate) {
        failureReason = 'operation success rate is below the minimum'
      } else if (p95LimitMs !== null && metrics.p95Ms > p95LimitMs) {
        failureReason = 'p95 latency exceeds the configured limit'
      }

      if (failureReason.length > 0) {
        return {
          result: 'failed',
          reason: failureReason,
          runnerExitCode,
          summaryPath: `data/results/${summaryName}`,
          p95Ms: metrics.p95Ms,
          successRate: metrics.successRate,
          droppedIterations: metrics.droppedIterations,
          completedIterations: metrics.completedIterations,
          cleanupRate: metrics.cleanupRate,
        }
      }

      if (runnerExitCode !== 0) {
        return {
          result: 'infrastructure_failure',
          reason: 'k6 exited unsuccessfully even though measured criteria passed',
          runnerExitCode,
          summaryPath: `data/results/${summaryName}`,
          p95Ms: metrics.p95Ms,
          successRate: metrics.successRate,
          droppedIterations: metrics.droppedIterations,
          completedIterations: metrics.completedIterations,
          cleanupRate: metrics.cleanupRate,
        }
      }

      return {
        result: 'passed',
        reason: '',
        runnerExitCode,
        summaryPath: `data/results/${summaryName}`,
        p95Ms: metrics.p95Ms,
        successRate: metrics.successRate,
        droppedIterations: metrics.droppedIterations,
        completedIterations: metrics.completedIterations,
        cleanupRate: metrics.cleanupRate,
      }
    },
  }
}
