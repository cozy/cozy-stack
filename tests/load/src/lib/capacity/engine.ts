import { randomUUID } from 'node:crypto'
import {
  existsSync,
  mkdirSync,
  renameSync,
  unlinkSync,
  writeFileSync,
} from 'node:fs'
import { join } from 'node:path'

import { computeP95LimitMs, getCapacityResultBound } from './metrics.ts'
import type {
  CapacityAttempt,
  CapacityPhase,
  CapacityReport,
  CapacityScenario,
  PointOutcome,
  ResultBound,
} from './types.ts'

export type BaseCapacityConfig = {
  campaignId: string
  targetUrl: string
  maxLevel: number
  baselineIterations: number
  confirmationIterations: number
  latencyMultiplier: number
  configuredP95LimitMs: number | null
  minimumSuccessRate: number
}

export type CapacityEngineOptions = {
  resultsDirectory: string
  reportFileName?: string
}

export type CampaignState<TConfig extends BaseCapacityConfig> = {
  attemptNumber: number
  config: TConfig
  report: CapacityReport
  reportPath: string
  temporaryReportPath: string
  scenario: CapacityScenario<TConfig>
}

export function saveJsonFile(filePath: string, value: unknown): string {
  const nextFilePath = `${filePath}.next`

  writeFileSync(nextFilePath, `${JSON.stringify(value, null, 2)}\n`, {
    encoding: 'utf8',
    mode: 0o600,
  })
  renameSync(nextFilePath, filePath)
  return filePath
}

export function makeCapacityReport<TConfig extends BaseCapacityConfig>(
  scenario: CapacityScenario<TConfig>,
  config: TConfig,
): CapacityReport {
  const report: CapacityReport = {
    campaign_id: config.campaignId,
    scenario: scenario.name,
    target: config.targetUrl,
    status: 'running',
    configured_max_level: config.maxLevel,
    criteria: {
      baseline_iterations: config.baselineIterations,
      maximum_p95_ms: config.configuredP95LimitMs,
      minimum_success_rate: config.minimumSuccessRate,
      maximum_dropped_iterations: 0,
      p95_baseline_multiplier:
        config.configuredP95LimitMs === null ? config.latencyMultiplier : null,
      p95_limit_mode:
        config.configuredP95LimitMs === null
          ? 'baseline_multiplier'
          : 'absolute',
      confirmation_iterations_per_vu: config.confirmationIterations,
    },
    attempts: [],
  }

  if (scenario.buildReportMetadata) {
    Object.assign(report, scenario.buildReportMetadata(config))
  }

  return report
}

export function makeCampaignState<TConfig extends BaseCapacityConfig>(
  scenario: CapacityScenario<TConfig>,
  config: TConfig,
  options: CapacityEngineOptions,
): CampaignState<TConfig> {
  mkdirSync(options.resultsDirectory, { recursive: true })

  const reportFileName =
    options.reportFileName ?? `${config.campaignId}-${scenario.name}-capacity.json`

  const state: CampaignState<TConfig> = {
    attemptNumber: 0,
    config,
    scenario,
    report: makeCapacityReport(scenario, config),
    reportPath: join(options.resultsDirectory, reportFileName),
    temporaryReportPath: join(
      options.resultsDirectory,
      `.${scenario.name}-capacity-${randomUUID()}`,
    ),
  }

  saveJsonFile(state.temporaryReportPath, state.report)
  return state
}

export function appendAttempt<TConfig extends BaseCapacityConfig>(
  state: CampaignState<TConfig>,
  attempt: CapacityAttempt,
): CapacityAttempt {
  state.report.attempts.push(attempt)
  saveJsonFile(state.temporaryReportPath, state.report)
  return attempt
}

export function runCapacityPoint<TConfig extends BaseCapacityConfig>(
  state: CampaignState<TConfig>,
  phase: CapacityPhase,
  level: number,
  iterationsPerVu: number,
  p95LimitMs: number | null,
): PointOutcome {
  state.attemptNumber += 1

  const { config, scenario } = state
  const executionId = `${config.campaignId}-${phase}-l${level}-a${state.attemptNumber}`

  const runResult = scenario.runPoint(
    phase,
    level,
    iterationsPerVu,
    p95LimitMs,
    config,
    state.attemptNumber,
    executionId,
  )

  const attempt: CapacityAttempt = {
    phase,
    level,
    vus: level,
    iterations_per_vu: iterationsPerVu,
    p95_ms: runResult.p95Ms,
    success_rate: runResult.successRate,
    dropped_iterations: runResult.droppedIterations,
    completed_iterations: runResult.completedIterations,
    cleanup_rate: runResult.cleanupRate,
    passed: runResult.result === 'passed',
    result: runResult.result,
    reason: runResult.reason,
    runner_exit_code: runResult.runnerExitCode,
    summary_path: runResult.summaryPath,
  }

  appendAttempt(state, attempt)
  return { p95Ms: runResult.p95Ms, result: runResult.result }
}

export function evaluateBurstLevel<TConfig extends BaseCapacityConfig>(
  state: CampaignState<TConfig>,
  level: number,
  p95LimitMs: number,
): PointOutcome {
  const firstOutcome = runCapacityPoint(state, 'discovery', level, 1, p95LimitMs)

  if (
    firstOutcome.result === 'passed' ||
    firstOutcome.result === 'infrastructure_failure'
  ) {
    return firstOutcome
  }

  process.stdout.write(`Retrying failing discovery point at level ${level}\n`)

  const secondOutcome = runCapacityPoint(
    state,
    'discovery-retry',
    level,
    1,
    p95LimitMs,
  )

  if (secondOutcome.result === 'infrastructure_failure') {
    return secondOutcome
  }
  if (secondOutcome.result === firstOutcome.result) {
    return firstOutcome
  }

  process.stdout.write(`Running deciding discovery point at level ${level}\n`)
  return runCapacityPoint(state, 'discovery-decider', level, 1, p95LimitMs)
}

export function evaluateConfirmationLevel<TConfig extends BaseCapacityConfig>(
  state: CampaignState<TConfig>,
  level: number,
  p95LimitMs: number,
): PointOutcome {
  return runCapacityPoint(
    state,
    'confirmation',
    level,
    state.config.confirmationIterations,
    p95LimitMs,
  )
}

export function finishReport<TConfig extends BaseCapacityConfig>(
  state: CampaignState<TConfig>,
  baselineP95Ms: number,
  p95LimitMs: number,
  maximumCapacity: number,
  bound: ResultBound,
): string {
  state.report.status = 'completed'
  state.report.baseline_p95_ms = baselineP95Ms
  state.report.p95_limit_ms = p95LimitMs
  state.report.maximum_capacity = maximumCapacity
  state.report.maximum_concurrent_uploads = maximumCapacity
  state.report.result_bound = bound
  saveJsonFile(state.temporaryReportPath, state.report)
  renameSync(state.temporaryReportPath, state.reportPath)

  const unit = state.scenario.unit
  if (bound === 'lower_bound') {
    process.stdout.write(
      `\nResult for ${state.scenario.name}: maximum capacity >= ${maximumCapacity} ${unit}\n`,
    )
  } else {
    process.stdout.write(
      `\nResult for ${state.scenario.name}: maximum capacity = ${maximumCapacity} ${unit}\n`,
    )
  }
  process.stdout.write(`Capacity report: ${state.reportPath}\n`)
  return state.reportPath
}

export function abortCampaign<TConfig extends BaseCapacityConfig>(
  state: CampaignState<TConfig>,
  reason: string,
): number {
  state.report.status = 'infrastructure_failure'
  state.report.failure_reason = reason
  saveJsonFile(state.temporaryReportPath, state.report)
  renameSync(state.temporaryReportPath, state.reportPath)
  process.stderr.write(`Capacity report: ${state.reportPath}\n`)
  process.stderr.write(`Capacity campaign stopped: ${reason}\n`)
  return 2
}

export function cleanupTemporaryReport(
  state: CampaignState<BaseCapacityConfig> | null,
): boolean {
  if (state === null) {
    return false
  }

  let removedFile = false

  for (const filePath of [
    state.temporaryReportPath,
    `${state.temporaryReportPath}.next`,
  ]) {
    if (existsSync(filePath)) {
      unlinkSync(filePath)
      removedFile = true
    }
  }

  return removedFile
}

export function runCapacitySearch<TConfig extends BaseCapacityConfig>(
  state: CampaignState<TConfig>,
): number {
  const { config, scenario } = state

  process.stdout.write(`Capacity campaign: ${config.campaignId}\n`)
  process.stdout.write(
    `Scenario: ${scenario.name}; maximum search level: ${config.maxLevel} ${scenario.unit}\n`,
  )

  const baselineOutcome = runCapacityPoint(
    state,
    'baseline',
    1,
    config.baselineIterations,
    null,
  )

  if (baselineOutcome.result === 'infrastructure_failure') {
    return abortCampaign(
      state,
      'baseline failed because of test infrastructure',
    )
  }
  if (baselineOutcome.p95Ms === null) {
    return abortCampaign(state, 'baseline produced no p95 measurement')
  }

  const baselineP95Ms = baselineOutcome.p95Ms
  const p95LimitMs = computeP95LimitMs(
    baselineP95Ms,
    config.latencyMultiplier,
    config.configuredP95LimitMs,
  )

  if (baselineOutcome.result === 'failed') {
    finishReport(state, baselineP95Ms, p95LimitMs, 0, 'exact')
    return 1
  }

  let lastPassingLevel = 1
  let firstFailingLevel = 0
  let currentLevel = 2

  while (currentLevel <= config.maxLevel) {
    const burstOutcome = evaluateBurstLevel(state, currentLevel, p95LimitMs)

    if (burstOutcome.result === 'infrastructure_failure') {
      return abortCampaign(
        state,
        `discovery failed because of test infrastructure at level ${currentLevel}`,
      )
    }

    if (burstOutcome.result === 'passed') {
      lastPassingLevel = currentLevel

      if (currentLevel === config.maxLevel) {
        break
      }

      currentLevel = Math.min(currentLevel * 2, config.maxLevel)
    } else {
      firstFailingLevel = currentLevel
      break
    }
  }

  if (firstFailingLevel > 0) {
    let searchLow = lastPassingLevel + 1
    let searchHigh = firstFailingLevel - 1

    while (searchLow <= searchHigh) {
      const searchMid = Math.floor((searchLow + searchHigh) / 2)
      const burstOutcome = evaluateBurstLevel(state, searchMid, p95LimitMs)

      if (burstOutcome.result === 'infrastructure_failure') {
        return abortCampaign(
          state,
          `binary discovery failed because of test infrastructure at level ${searchMid}`,
        )
      }

      if (burstOutcome.result === 'passed') {
        lastPassingLevel = searchMid
        searchLow = searchMid + 1
      } else {
        searchHigh = searchMid - 1
      }
    }
  }

  const candidateLevel = lastPassingLevel
  const confirmationOutcome = evaluateConfirmationLevel(
    state,
    candidateLevel,
    p95LimitMs,
  )

  if (confirmationOutcome.result === 'infrastructure_failure') {
    return abortCampaign(
      state,
      `confirmation failed because of test infrastructure at level ${candidateLevel}`,
    )
  }

  if (confirmationOutcome.result === 'passed') {
    const resultBound = getCapacityResultBound(
      candidateLevel,
      config.maxLevel,
      firstFailingLevel,
    )

    finishReport(
      state,
      baselineP95Ms,
      p95LimitMs,
      candidateLevel,
      resultBound,
    )
    return 0
  }

  let confirmedLevel = 0
  let searchLow = 1
  let searchHigh = candidateLevel - 1

  while (searchLow <= searchHigh) {
    const searchMid = Math.floor((searchLow + searchHigh) / 2)
    const searchOutcome = evaluateConfirmationLevel(
      state,
      searchMid,
      p95LimitMs,
    )

    if (searchOutcome.result === 'infrastructure_failure') {
      return abortCampaign(
        state,
        `confirmation search failed because of test infrastructure at level ${searchMid}`,
      )
    }

    if (searchOutcome.result === 'passed') {
      confirmedLevel = searchMid
      searchLow = searchMid + 1
    } else {
      searchHigh = searchMid - 1
    }
  }

  finishReport(state, baselineP95Ms, p95LimitMs, confirmedLevel, 'exact')
  return confirmedLevel === 0 ? 1 : 0
}
