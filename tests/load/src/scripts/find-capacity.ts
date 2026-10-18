import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import {
  getUploadDataSizeBytes,
  isUploadDataSize,
  uploadDataSizes,
  type UploadDataSize,
} from '../fixtures/upload-sizes.ts'
import {
  cleanupTemporaryReport,
  createSharedDriveActivityCapacityScenario,
  createUploadCapacityScenario,
  defaultMaximumVirtualUsers,
  makeCampaignState,
  runCapacitySearch,
  type CampaignState,
  type SharedDriveCampaignConfig,
  type UploadCampaignConfig,
} from '../lib/capacity/index.ts'
import { getUploadTarget } from '../lib/upload-target.ts'

const scriptDirectory = dirname(fileURLToPath(import.meta.url))
const harnessDirectory = resolve(scriptDirectory, '..', '..')
const resultsDirectory = join(harnessDirectory, 'data', 'results')

function getPositiveInteger(environmentName: string, value: string): number {
  if (!/^[1-9][0-9]*$/.test(value)) {
    throw new Error(`${environmentName} must be a positive integer`)
  }

  const parsedValue = Number(value)
  if (!Number.isSafeInteger(parsedValue)) {
    throw new Error(`${environmentName} must be a safe positive integer`)
  }

  return parsedValue
}

function getPositiveNumber(environmentName: string, value: string): number {
  const parsedValue = Number(value)
  if (!Number.isFinite(parsedValue) || parsedValue <= 0) {
    throw new Error(`${environmentName} must be a positive number`)
  }

  return parsedValue
}

function getOptionalPositiveNumber(
  environmentName: string,
  value: string | undefined,
): number | null {
  if (!value) {
    return null
  }
  return getPositiveNumber(environmentName, value)
}

function getMinimumSuccessRate(value: string): number {
  const parsedValue = Number(value)
  if (!Number.isFinite(parsedValue) || parsedValue <= 0 || parsedValue > 1) {
    throw new Error('MIN_SUCCESS_RATE must be greater than 0 and at most 1')
  }
  return parsedValue
}

function makeDefaultCampaignId(prefix: string): string {
  const timestamp = new Date()
    .toISOString()
    .replace(/[-:]/g, '')
    .replace(/\.\d{3}/, '')

  return `${timestamp}-${prefix}-${process.pid}`
}

export function runCapacityCampaign(): number {
  let state: CampaignState<any> | null = null

  try {
    const scenarioName = (process.env.SCENARIO || 'upload').toLowerCase()
    const baseUrl = process.env.BASE_URL || 'http://mock'
    const confirmationIterations = getPositiveInteger(
      'CONFIRMATION_ITERATIONS',
      process.env.CONFIRMATION_ITERATIONS || '20',
    )
    const baselineIterations = getPositiveInteger(
      'BASELINE_ITERATIONS',
      process.env.BASELINE_ITERATIONS || '50',
    )
    const latencyMultiplier = getPositiveNumber(
      'LATENCY_MULTIPLIER',
      process.env.LATENCY_MULTIPLIER || '2',
    )
    const configuredP95LimitMs = getOptionalPositiveNumber(
      'P95_LIMIT_MS',
      process.env.P95_LIMIT_MS,
    )
    const minimumSuccessRate = getMinimumSuccessRate(
      process.env.MIN_SUCCESS_RATE || '0.99',
    )

    if (scenarioName === 'upload') {
      const configuredFileSize = process.env.FILE_SIZE ?? '1K'
      if (!isUploadDataSize(configuredFileSize)) {
        throw new Error(
          `Unknown FILE_SIZE ${configuredFileSize}. Expected one of: ${uploadDataSizes.join(' ')}`,
        )
      }

      const configuredMaxVus = process.env.MAX_VUS
      const maxVus = getPositiveInteger(
        'MAX_VUS',
        configuredMaxVus && configuredMaxVus.length > 0
          ? configuredMaxVus
          : String(defaultMaximumVirtualUsers[configuredFileSize]),
      )

      const uploadConfig: UploadCampaignConfig = {
        baselineIterations,
        campaignId:
          process.env.CAPACITY_RUN_ID || makeDefaultCampaignId('upload-capacity'),
        confirmationIterations,
        configuredP95LimitMs,
        consistencyChunkSizeBytes: getPositiveInteger(
          'CONSISTENCY_CHUNK_SIZE_BYTES',
          process.env.CONSISTENCY_CHUNK_SIZE_BYTES || String(16 * 1024 * 1024),
        ),
        fileSize: configuredFileSize,
        fileSizeBytes: getUploadDataSizeBytes(configuredFileSize),
        forceCapacityRun: process.env.FORCE_CAPACITY_RUN === '1',
        latencyMultiplier,
        maxLevel: maxVus,
        maxVus,
        minimumSuccessRate,
        targetUrl: baseUrl,
        uploadTarget: getUploadTarget(process.env),
      }

      const scenario = createUploadCapacityScenario(
        harnessDirectory,
        resultsDirectory,
      )
      scenario.validatePreflight?.(uploadConfig)

      state = makeCampaignState(scenario, uploadConfig, {
        resultsDirectory,
        reportFileName: `${uploadConfig.campaignId}-upload-capacity.json`,
      })
      return runCapacitySearch(state)
    }

    if (scenarioName === 'shared-drive-activity' || scenarioName === 'shared-drive') {
      const configuredFileSize = process.env.FILE_SIZE ?? '100K'
      if (!isUploadDataSize(configuredFileSize)) {
        throw new Error(
          `Unknown FILE_SIZE ${configuredFileSize}. Expected one of: ${uploadDataSizes.join(' ')}`,
        )
      }

      const maxLevel = getPositiveInteger(
        'MAX_SCALE',
        process.env.MAX_SCALE || process.env.MAX_VUS || '16',
      )
      const sharedDriveId = process.env.SHARED_DRIVE_ID || ''
      const sharedDriveRootId = process.env.SHARED_DRIVE_ROOT_ID || ''

      const sharedDriveConfig: SharedDriveCampaignConfig = {
        baselineIterations: getPositiveInteger(
          'BASELINE_ITERATIONS',
          process.env.BASELINE_ITERATIONS || '5',
        ),
        campaignId:
          process.env.CAPACITY_RUN_ID ||
          makeDefaultCampaignId('shared-drive-capacity'),
        confirmationIterations: getPositiveInteger(
          'CONFIRMATION_ITERATIONS',
          process.env.CONFIRMATION_ITERATIONS || '10',
        ),
        configuredP95LimitMs,
        duration: process.env.DURATION || '20s',
        fileSize: configuredFileSize,
        latencyMultiplier,
        maxLevel,
        minimumSuccessRate: getMinimumSuccessRate(
          process.env.MIN_SUCCESS_RATE || '0.95',
        ),
        sharedDriveId,
        sharedDriveRootId,
        targetUrl: baseUrl,
        writerRatio: getPositiveInteger('WRITER_RATIO', process.env.WRITER_RATIO || '1'),
        wsRatio: getPositiveInteger('WS_RATIO', process.env.WS_RATIO || '3'),
        changesRatio: getPositiveInteger('CHANGES_RATIO', process.env.CHANGES_RATIO || '2'),
      }

      const scenario = createSharedDriveActivityCapacityScenario(
        harnessDirectory,
        resultsDirectory,
      )
      scenario.validatePreflight?.(sharedDriveConfig)

      state = makeCampaignState(scenario, sharedDriveConfig, {
        resultsDirectory,
        reportFileName: `${sharedDriveConfig.campaignId}-shared-drive-capacity.json`,
      })
      return runCapacitySearch(state)
    }

    throw new Error(
      `Unknown SCENARIO "${scenarioName}". Expected "upload" or "shared-drive-activity"`,
    )
  } catch (error: unknown) {
    const reason = error instanceof Error ? error.message : String(error)

    if (state !== null) {
      state.report.status = 'infrastructure_failure'
      state.report.failure_reason = reason
      process.stderr.write(`Capacity campaign stopped: ${reason}\n`)
      return 2
    }

    process.stderr.write(`${reason}\n`)
    return 2
  } finally {
    cleanupTemporaryReport(state)
  }
}

function isMainModule(): boolean {
  const entryPath = process.argv[1]
  return (
    entryPath !== undefined &&
    resolve(entryPath) === fileURLToPath(import.meta.url)
  )
}

if (isMainModule()) {
  process.exitCode = runCapacityCampaign()
}
