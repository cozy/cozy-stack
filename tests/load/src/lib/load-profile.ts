import { parseDurationToMs } from './duration.ts'
import { parsePositiveIntegerEnv } from './env.ts'

// A ramp holds the shared-drive mix at increasing levels. Level L runs
// L x vusPerLevel VUs in each worker group. Every sample is tagged with the
// level being held, so one k6 run can judge each level separately.

const RAMP_TRANSITION_MS = 10_000
const MINIMUM_HOLD_MS = 5_000
const DEFAULT_RAMP_LEVELS = '1,2,4,8,16'

export type RampVusPerLevel = {
  changesReaders: number
  writers: number
  wsReaders: number
}

export type RampPlan = {
  holdMs: number
  levels: number[]
  transitionMs: number
  vusPerLevel: RampVusPerLevel
}

export type RampPhase = {
  endsAtMs: number
  level: number | null
}

export type RampCriteria = {
  latencyMultiplier: number
  minimumSuccessRate: number
  p95LimitMs: number | null
}

export type RampingVuStage = {
  duration: string
  target: number
}

export type ThresholdDefinition =
  | string
  | { abortOnFail?: boolean; delayAbortEval?: string; threshold: string }

export type SummaryMetrics = Record<
  string,
  { values?: Record<string, number> } | undefined
>

export type RampLevelStatus = 'passed' | 'failed' | 'not_measured' | 'not_run'
export type RampResultBound = 'exact' | 'lower_bound' | 'inconclusive'

export type RampLevelResult = {
  changes_p95_ms: number | null
  changes_success_rate: number | null
  level: number
  reasons: string[]
  status: RampLevelStatus
  upload_p95_ms: number | null
  upload_success_rate: number | null
  ws_auth_rate: number | null
  ws_handshake_rate: number | null
}

export type RampReport = {
  criteria: {
    baseline_level: number
    changes_p95_limit_ms: number | null
    minimum_success_rate: number
    p95_baseline_multiplier: number | null
    p95_limit_mode: 'absolute' | 'baseline_multiplier'
    upload_p95_limit_ms: number | null
  }
  levels: RampLevelResult[]
  max_sustained_level: number
  plan: {
    hold_ms: number
    levels: number[]
    transition_ms: number
    vus_per_level: RampVusPerLevel
  }
  result_bound: RampResultBound
}

type Environment = Record<string, string | undefined>

type RateKey =
  | 'changes_success_rate'
  | 'upload_success_rate'
  | 'ws_auth_rate'
  | 'ws_handshake_rate'

type TrendKey = 'changes_p95_ms' | 'upload_p95_ms'

type JudgedRate = { key: RateKey; label: string; metric: string }
type JudgedTrend = {
  key: TrendKey
  label: string
  metric: string
  tags: Record<string, string>
}

const judgedRates: readonly JudgedRate[] = [
  {
    key: 'upload_success_rate',
    label: 'upload success rate',
    metric: 'shared_drive_upload_success_rate',
  },
  {
    key: 'changes_success_rate',
    label: 'changes success rate',
    metric: 'shared_drive_changes_success_rate',
  },
  {
    key: 'ws_handshake_rate',
    label: 'ws handshake rate',
    metric: 'shared_drive_ws_handshake_rate',
  },
  { key: 'ws_auth_rate', label: 'ws auth rate', metric: 'shared_drive_ws_auth_rate' },
]

const judgedTrends: readonly JudgedTrend[] = [
  {
    key: 'upload_p95_ms',
    label: 'upload p95',
    metric: 'http_req_duration',
    tags: { operation: 'file-upload' },
  },
  {
    key: 'changes_p95_ms',
    label: 'changes p95',
    metric: 'shared_drive_changes_duration_ms',
    tags: {},
  },
]

export function parseRampLevels(raw: string): number[] {
  const parts = raw.split(',').map((part: string): string => part.trim())
  const levels = parts.map(Number)
  const isValid =
    raw.trim().length > 0 &&
    parts.every((part: string): boolean => /^[1-9][0-9]*$/.test(part)) &&
    levels.every(
      (level: number, index: number): boolean =>
        index === 0 || level > (levels[index - 1] ?? 0),
    )

  if (!isValid) {
    throw new Error(
      `RAMP_LEVELS must be strictly increasing positive integers, got "${raw}"`,
    )
  }

  return levels
}

export function parseRampPlan(env: Environment): RampPlan {
  const rawStepDuration = env.RAMP_STEP_DURATION || '1m'
  const holdMs = parseDurationToMs(rawStepDuration)

  if (holdMs < MINIMUM_HOLD_MS) {
    throw new Error(
      `RAMP_STEP_DURATION must be at least 5s, got "${rawStepDuration}"`,
    )
  }

  return {
    holdMs,
    levels: parseRampLevels(env.RAMP_LEVELS || DEFAULT_RAMP_LEVELS),
    transitionMs: RAMP_TRANSITION_MS,
    vusPerLevel: {
      changesReaders: parsePositiveIntegerEnv(
        env,
        'RAMP_CHANGES_READERS_PER_LEVEL',
        2,
      ),
      writers: parsePositiveIntegerEnv(env, 'RAMP_WRITERS_PER_LEVEL', 1),
      wsReaders: parsePositiveIntegerEnv(env, 'RAMP_WS_READERS_PER_LEVEL', 3),
    },
  }
}

function parsePositiveNumber(name: string, raw: string): number {
  const value = Number(raw)

  if (!Number.isFinite(value) || value <= 0) {
    throw new Error(`${name} must be a positive number, got "${raw}"`)
  }

  return value
}

export function parseRampCriteria(env: Environment): RampCriteria {
  const rawSuccessRate = env.MIN_SUCCESS_RATE || '0.99'
  const minimumSuccessRate = Number(rawSuccessRate)

  if (
    !Number.isFinite(minimumSuccessRate) ||
    minimumSuccessRate <= 0 ||
    minimumSuccessRate > 1
  ) {
    throw new Error(
      `MIN_SUCCESS_RATE must be greater than 0 and at most 1, got "${rawSuccessRate}"`,
    )
  }

  return {
    latencyMultiplier: parsePositiveNumber(
      'LATENCY_MULTIPLIER',
      env.LATENCY_MULTIPLIER || '2',
    ),
    minimumSuccessRate,
    p95LimitMs: env.P95_LIMIT_MS
      ? parsePositiveNumber('P95_LIMIT_MS', env.P95_LIMIT_MS)
      : null,
  }
}

function getLevelSpanMs(plan: RampPlan): number {
  return plan.transitionMs + plan.holdMs
}

export function getRampTotalDurationMs(plan: RampPlan): number {
  return plan.levels.length * getLevelSpanMs(plan)
}

export function buildRampingVuStages(
  plan: RampPlan,
  vusPerLevel: number,
): RampingVuStage[] {
  return plan.levels.flatMap((level: number): RampingVuStage[] => {
    const target = level * vusPerLevel
    return [
      { duration: `${plan.transitionMs}ms`, target },
      { duration: `${plan.holdMs}ms`, target },
    ]
  })
}

export function getRampPhaseAt(plan: RampPlan, elapsedMs: number): RampPhase {
  const spanMs = getLevelSpanMs(plan)
  const index = Math.floor(elapsedMs / spanMs)
  const level = plan.levels[index]

  if (elapsedMs < 0 || level === undefined) {
    return { endsAtMs: getRampTotalDurationMs(plan), level: null }
  }

  const transitionEndsAtMs = index * spanMs + plan.transitionMs

  return elapsedMs < transitionEndsAtMs
    ? { endsAtMs: transitionEndsAtMs, level: null }
    : { endsAtMs: (index + 1) * spanMs, level }
}

export function rampLevelMetricName(
  metric: string,
  level: number,
  tags: Record<string, string> = {},
): string {
  const selectors = [
    ...Object.entries(tags).map(
      ([name, value]: [string, string]): string => `${name}:${value}`,
    ),
    `level:${level}`,
  ]
  return `${metric}{${selectors.join(',')}}`
}

// The per-level thresholds always pass. They exist so k6 keeps a submetric
// for each level; the summary judges those submetrics after the run.
export function buildRampThresholds(
  plan: RampPlan,
): Record<string, ThresholdDefinition[]> {
  const thresholds: Record<string, ThresholdDefinition[]> = {
    cleanup_success: ['rate==1'],
    // Stop hammering a stack that is clearly down.
    http_req_failed: [
      { abortOnFail: true, delayAbortEval: '30s', threshold: 'rate<0.5' },
    ],
  }

  for (const level of plan.levels) {
    for (const { metric } of judgedRates) {
      thresholds[rampLevelMetricName(metric, level)] = ['rate>=0']
    }
    for (const { metric, tags } of judgedTrends) {
      thresholds[rampLevelMetricName(metric, level, tags)] = ['p(95)>=0']
    }
  }

  return thresholds
}

function readRate(metrics: SummaryMetrics, name: string): number | null {
  const values = metrics[name]?.values
  const samples = (values?.passes ?? 0) + (values?.fails ?? 0)
  const rate = values?.rate

  return samples > 0 && typeof rate === 'number' && Number.isFinite(rate)
    ? rate
    : null
}

// k6 reports an empty submetric with a p95 of 0, so a p95 only counts when
// the trend also has samples. The ramp asks k6 for `count` for that reason.
function readP95(metrics: SummaryMetrics, name: string): number | null {
  const values = metrics[name]?.values
  const value = values?.['p(95)']

  return (values?.count ?? 0) > 0 &&
    typeof value === 'number' &&
    Number.isFinite(value)
    ? value
    : null
}

function round(value: number, digits: number): number {
  return Number(value.toFixed(digits))
}

type LatencyLimits = Record<TrendKey, number | null>

function getLatencyLimits(
  metrics: SummaryMetrics,
  plan: RampPlan,
  criteria: RampCriteria,
): LatencyLimits {
  const baselineLevel = plan.levels[0] ?? 1
  const limitFor = (trend: JudgedTrend): number | null => {
    if (criteria.p95LimitMs !== null) {
      return criteria.p95LimitMs
    }

    const baseline = readP95(
      metrics,
      rampLevelMetricName(trend.metric, baselineLevel, trend.tags),
    )
    return baseline === null ? null : baseline * criteria.latencyMultiplier
  }

  return Object.fromEntries(
    judgedTrends.map((trend: JudgedTrend) => [trend.key, limitFor(trend)]),
  ) as LatencyLimits
}

function evaluateLevel(
  metrics: SummaryMetrics,
  level: number,
  criteria: RampCriteria,
  limits: LatencyLimits,
): RampLevelResult {
  const rates = Object.fromEntries(
    judgedRates.map(({ key, metric }) => [
      key,
      readRate(metrics, rampLevelMetricName(metric, level)),
    ]),
  ) as Record<RateKey, number | null>
  const trends = Object.fromEntries(
    judgedTrends.map(({ key, metric, tags }) => [
      key,
      readP95(metrics, rampLevelMetricName(metric, level, tags)),
    ]),
  ) as Record<TrendKey, number | null>
  const result: RampLevelResult = {
    ...rates,
    ...trends,
    level,
    reasons: [],
    status: 'passed',
  }

  if (rates.upload_success_rate === null && rates.changes_success_rate === null) {
    return { ...result, status: 'not_run' }
  }

  const failures: string[] = []
  const missing: string[] = []

  for (const { key, label } of judgedRates) {
    const value = rates[key]
    if (value === null) {
      missing.push(`${label} has no samples`)
    } else if (value < criteria.minimumSuccessRate) {
      failures.push(
        `${label} ${round(value, 4)} is below ${criteria.minimumSuccessRate}`,
      )
    }
  }

  for (const { key, label } of judgedTrends) {
    const value = trends[key]
    const limit = limits[key]
    if (value === null) {
      missing.push(`${label} has no samples`)
    } else if (limit === null) {
      missing.push(`${label} has no baseline to compare with`)
    } else if (value > limit) {
      failures.push(
        `${label} ${round(value, 1)} ms exceeds ${round(limit, 1)} ms`,
      )
    }
  }

  const status: RampLevelStatus =
    failures.length > 0 ? 'failed' : missing.length > 0 ? 'not_measured' : 'passed'

  return { ...result, reasons: [...failures, ...missing], status }
}

export function evaluateRampLevels(
  metrics: SummaryMetrics,
  plan: RampPlan,
  criteria: RampCriteria,
): RampReport {
  const limits = getLatencyLimits(metrics, plan, criteria)
  const levels = plan.levels.map(
    (level: number): RampLevelResult =>
      evaluateLevel(metrics, level, criteria, limits),
  )
  const firstNotPassed = levels.find(
    ({ status }: RampLevelResult): boolean => status !== 'passed',
  )
  const sustainedLevels =
    firstNotPassed === undefined
      ? levels
      : levels.slice(0, levels.indexOf(firstNotPassed))
  const maxSustainedLevel =
    sustainedLevels[sustainedLevels.length - 1]?.level ?? 0
  const resultBound: RampResultBound =
    firstNotPassed === undefined
      ? 'lower_bound'
      : firstNotPassed.status === 'failed'
        ? 'exact'
        : 'inconclusive'

  return {
    criteria: {
      baseline_level: plan.levels[0] ?? 1,
      changes_p95_limit_ms: limits.changes_p95_ms,
      minimum_success_rate: criteria.minimumSuccessRate,
      p95_baseline_multiplier:
        criteria.p95LimitMs === null ? criteria.latencyMultiplier : null,
      p95_limit_mode: criteria.p95LimitMs === null ? 'baseline_multiplier' : 'absolute',
      upload_p95_limit_ms: limits.upload_p95_ms,
    },
    levels,
    max_sustained_level: maxSustainedLevel,
    plan: {
      hold_ms: plan.holdMs,
      levels: plan.levels,
      transition_ms: plan.transitionMs,
      vus_per_level: plan.vusPerLevel,
    },
    result_bound: resultBound,
  }
}

export function formatRampReport(report: RampReport): string {
  const { changesReaders, writers, wsReaders } = report.plan.vus_per_level
  const lines = [
    '',
    `Shared drive ramp (writers:ws readers:changes readers per level = ${writers}:${wsReaders}:${changesReaders})`,
    ...report.levels.map(({ level, reasons, status }: RampLevelResult): string =>
      reasons.length > 0
        ? `  level ${level}: ${status} (${reasons.join('; ')})`
        : `  level ${level}: ${status}`,
    ),
    `Maximum sustained level: ${report.max_sustained_level} (${report.result_bound})`,
    '',
  ]

  return lines.join('\n')
}
