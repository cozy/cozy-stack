import { spawnSync } from 'node:child_process'
import { existsSync, readFileSync, statfsSync, statSync } from 'node:fs'
import { join } from 'node:path'

import {
  getUploadBinaryFilename,
  getUploadDataSizeBytes,
  isUploadDataSize,
  uploadDataSizes,
  type UploadDataSize,
} from '../../../fixtures/upload-sizes.ts'
import {
  getUploadTarget,
  type UploadTarget,
  type UploadTargetKind,
} from '../../upload-target.ts'
import type { BaseCapacityConfig } from '../engine.ts'
import {
  getMetricNumber,
  isInterruptedCapacityPoint,
  isJsonRecord,
} from '../metrics.ts'
import type {
  CapacityPhase,
  CapacityScenario,
  PointResult,
  PointRunResult,
} from '../types.ts'

const MINIMUM_FREE_DISK_BYTES = 5n * 1024n * 1024n * 1024n
const ONE_GIBIBYTE = 1024n * 1024n * 1024n

export type UploadCampaignConfig = BaseCapacityConfig & {
  consistencyChunkSizeBytes: number
  fileSize: UploadDataSize
  fileSizeBytes: number
  forceCapacityRun: boolean
  maxVus: number
  uploadTarget: UploadTarget
}

type PointMetrics = {
  cleanupRate: number
  completedIterations: number
  droppedIterations: number
  p95Ms: number
  successRate: number
}

export const defaultMaximumVirtualUsers: Record<UploadDataSize, number> = {
  '1K': 256,
  '100K': 256,
  '1M': 128,
  '10M': 32,
  '100M': 8,
  '1G': 2,
}

export function getPointMetrics(summaryPath: string): PointMetrics | null {
  try {
    const parsedSummary: unknown = JSON.parse(readFileSync(summaryPath, 'utf8'))

    if (!isJsonRecord(parsedSummary)) {
      return null
    }

    const p95Ms = getMetricNumber(
      parsedSummary,
      ['http_req_duration{operation:file-upload}', 'http_req_duration'],
      'p(95)',
    )
    const successRate = getMetricNumber(
      parsedSummary,
      ['upload_success{operation:file-upload}', 'upload_success'],
      'value',
    )
    const completedIterations = getMetricNumber(
      parsedSummary,
      ['iterations'],
      'count',
    )
    const cleanupRate = getMetricNumber(
      parsedSummary,
      ['cleanup_success'],
      'value',
    )
    const droppedIterations =
      getMetricNumber(parsedSummary, ['dropped_iterations'], 'count') ?? 0

    if (
      p95Ms === null ||
      successRate === null ||
      completedIterations === null ||
      cleanupRate === null
    ) {
      return null
    }

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

export function ensureResourceCapacity(
  config: UploadCampaignConfig,
  harnessDirectory: string,
): true {
  const uploadBinaryPath = join(
    harnessDirectory,
    'data',
    'upload-binaries',
    getUploadBinaryFilename(config.fileSize),
  )
  const availableDiskStats = statfsSync(harnessDirectory, { bigint: true })
  const availableDiskBytes =
    BigInt(availableDiskStats.bavail) * BigInt(availableDiskStats.bsize)
  const uploadBinaryBytes =
    existsSync(uploadBinaryPath) &&
    statSync(uploadBinaryPath).size === config.fileSizeBytes
      ? 0n
      : BigInt(config.fileSizeBytes)
  const baselineBytes =
    BigInt(config.fileSizeBytes) * BigInt(config.baselineIterations)
  const confirmationBytes =
    BigInt(config.fileSizeBytes) *
    BigInt(config.maxVus) *
    BigInt(config.confirmationIterations)
  const projectedDiskBytes =
    uploadBinaryBytes +
    (baselineBytes > confirmationBytes ? baselineBytes : confirmationBytes)

  if (
    !config.forceCapacityRun &&
    availableDiskBytes - projectedDiskBytes < MINIMUM_FREE_DISK_BYTES
  ) {
    throw new Error(
      [
        'Projected campaign data would leave less than 5 GiB free.',
        `Available bytes: ${availableDiskBytes}; projected bytes: ${projectedDiskBytes}`,
        'Set FORCE_CAPACITY_RUN=1 only after verifying load-generator and Cozy storage.',
      ].join('\n'),
    )
  }

  const dockerInfo = spawnSync(
    'docker',
    ['info', '--format', '{{.MemTotal}}'],
    { encoding: 'utf8' },
  )
  const dockerMemoryOutput = dockerInfo.stdout.trim()

  if (dockerInfo.status !== 0 || !/^[1-9][0-9]*$/.test(dockerMemoryOutput)) {
    throw new Error('Could not determine Docker memory. Is Docker running?')
  }

  const dockerMemoryBytes = BigInt(dockerMemoryOutput)
  const consistencyBufferBytes = BigInt(
    Math.min(config.fileSizeBytes, config.consistencyChunkSizeBytes),
  )
  const projectedMemoryBytes =
    ONE_GIBIBYTE +
    (3n * BigInt(config.fileSizeBytes) + consistencyBufferBytes) *
      BigInt(config.maxVus)

  if (!config.forceCapacityRun && projectedMemoryBytes > dockerMemoryBytes) {
    throw new Error(
      [
        'Projected k6 memory exceeds the memory available to Docker.',
        `Docker bytes: ${dockerMemoryBytes}; projected bytes: ${projectedMemoryBytes}`,
        'Set FORCE_CAPACITY_RUN=1 only after verifying the load generator.',
      ].join('\n'),
    )
  }

  return true
}

export function createUploadCapacityScenario(
  harnessDirectory: string,
  resultsDirectory: string,
): CapacityScenario<UploadCampaignConfig> {
  return {
    name: 'upload',
    unit: 'concurrent uploads',
    validatePreflight(config: UploadCampaignConfig): void {
      ensureResourceCapacity(config, harnessDirectory)
    },
    buildReportMetadata(config: UploadCampaignConfig): Record<string, unknown> {
      return {
        upload_target: config.uploadTarget.kind,
        file_size: config.fileSize,
        file_size_bytes: config.fileSizeBytes,
        configured_max_vus: config.maxVus,
      }
    },
    runPoint(
      phase: CapacityPhase,
      vus: number,
      iterationsPerVu: number,
      p95LimitMs: number | null,
      config: UploadCampaignConfig,
      attemptNumber: number,
      executionId: string,
    ): PointRunResult {
      const summaryName = `${executionId}-summary.json`
      const summaryPath = join(resultsDirectory, summaryName)
      const expectedIterations = vus * iterationsPerVu

      process.stdout.write(
        `\nRunning ${phase} point: ${vus} VUs x ${iterationsPerVu} upload(s)\n`,
      )

      const makeArguments = [
        '-C',
        harnessDirectory,
        'upload',
        `BASE_URL=${config.targetUrl}`,
        `FILE_SIZE=${config.fileSize}`,
        `VUS=${vus}`,
        'UPLOAD_MODE=iterations',
        `ITERATIONS_PER_VU=${iterationsPerVu}`,
        `CONSISTENCY_CHUNK_SIZE_BYTES=${config.consistencyChunkSizeBytes}`,
        `CAPACITY_RUN_ID=${config.campaignId}`,
        `CAPACITY_PHASE=${phase}`,
        `MIN_SUCCESS_RATE=${config.minimumSuccessRate}`,
        `P95_LIMIT_MS=${p95LimitMs ?? ''}`,
        `EXECUTION_ID=${executionId}`,
        `UPLOAD_TARGET=${config.uploadTarget.kind}`,
      ]

      if (config.uploadTarget.kind === 'shared-drive') {
        makeArguments.push(
          `SHARED_DRIVE_ID=${config.uploadTarget.driveId}`,
          `SHARED_DRIVE_ROOT_ID=${config.uploadTarget.rootDirectoryId}`,
        )
      }

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

      const metrics = getPointMetrics(summaryPath)

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

      if (
        isInterruptedCapacityPoint(
          runnerExitCode,
          metrics.completedIterations,
          expectedIterations,
        )
      ) {
        return {
          result: 'infrastructure_failure',
          reason: 'k6 stopped before completing every upload consistency check',
          runnerExitCode,
          summaryPath: `data/results/${summaryName}`,
          p95Ms: metrics.p95Ms,
          successRate: metrics.successRate,
          droppedIterations: metrics.droppedIterations,
          completedIterations: metrics.completedIterations,
          cleanupRate: metrics.cleanupRate,
        }
      }

      if (metrics.completedIterations === 0) {
        return {
          result: 'infrastructure_failure',
          reason: 'k6 completed no upload iterations',
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

      if (
        metrics.completedIterations !== expectedIterations ||
        metrics.droppedIterations !== 0
      ) {
        failureReason = 'iterations were dropped or incomplete'
      } else if (metrics.successRate < config.minimumSuccessRate) {
        failureReason = 'upload success rate is below the minimum'
      } else if (p95LimitMs !== null && metrics.p95Ms > p95LimitMs) {
        failureReason = 'upload p95 exceeds the configured limit'
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
