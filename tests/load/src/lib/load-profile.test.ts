import { deepEqual, equal, match, throws } from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  buildRampThresholds,
  buildRampingVuStages,
  evaluateRampLevels,
  formatRampReport,
  getRampPhaseAt,
  getRampTotalDurationMs,
  parseRampCriteria,
  parseRampLevels,
  parseRampPlan,
  rampLevelMetricName,
  type RampCriteria,
  type RampPlan,
  type SummaryMetrics,
} from './load-profile.ts'

const plan: RampPlan = {
  holdMs: 60_000,
  levels: [1, 2, 4],
  transitionMs: 10_000,
  vusPerLevel: { changesReaders: 2, writers: 1, wsReaders: 3 },
}

const criteria: RampCriteria = {
  latencyMultiplier: 2,
  minimumSuccessRate: 0.99,
  p95LimitMs: null,
}

type LevelSample = {
  changesP95?: number | undefined
  changesSuccess?: number | undefined
  uploadP95?: number | undefined
  uploadSuccess?: number | undefined
  wsAuth?: number | undefined
  wsHandshake?: number | undefined
}

function rate(value: number | undefined): { values: Record<string, number> } {
  return value === undefined
    ? { values: { fails: 0, passes: 0, rate: 0 } }
    : { values: { fails: 1 - value, passes: value, rate: value } }
}

// k6 keeps an empty placeholder submetric with a p95 of 0 and a count of 0.
function trend(value: number | undefined): { values: Record<string, number> } {
  return value === undefined
    ? { values: { count: 0, 'p(95)': 0 } }
    : { values: { count: 10, 'p(95)': value } }
}

function makeMetrics(samples: Record<number, LevelSample>): SummaryMetrics {
  const metrics: SummaryMetrics = {}

  for (const [rawLevel, sample] of Object.entries(samples)) {
    const level = Number(rawLevel)
    metrics[rampLevelMetricName('shared_drive_upload_success_rate', level)] =
      rate(sample.uploadSuccess)
    metrics[rampLevelMetricName('shared_drive_changes_success_rate', level)] =
      rate(sample.changesSuccess)
    metrics[rampLevelMetricName('shared_drive_ws_handshake_rate', level)] =
      rate(sample.wsHandshake)
    metrics[rampLevelMetricName('shared_drive_ws_auth_rate', level)] = rate(
      sample.wsAuth,
    )
    metrics[
      rampLevelMetricName('http_req_duration', level, {
        operation: 'file-upload',
      })
    ] = trend(sample.uploadP95)
    metrics[rampLevelMetricName('shared_drive_changes_duration_ms', level)] =
      trend(sample.changesP95)
  }

  return metrics
}

const healthy: LevelSample = {
  changesP95: 50,
  changesSuccess: 1,
  uploadP95: 100,
  uploadSuccess: 1,
  wsAuth: 1,
  wsHandshake: 1,
}

describe('parseRampLevels', (): void => {
  it('parses strictly increasing positive integers', (): void => {
    deepEqual(parseRampLevels(' 1, 2 ,4,8 '), [1, 2, 4, 8])
  })

  it('rejects empty, non-integer, non-positive, or unordered levels', (): void => {
    for (const raw of ['', '1,,2', '1,2.5', '0,1', '-1', '2,2', '4,2', 'a']) {
      throws(() => parseRampLevels(raw), /RAMP_LEVELS/, raw)
    }
  })
})

describe('parseRampPlan', (): void => {
  it('uses the documented defaults', (): void => {
    deepEqual(parseRampPlan({}), {
      holdMs: 60_000,
      levels: [1, 2, 4, 8, 16],
      transitionMs: 10_000,
      vusPerLevel: { changesReaders: 2, writers: 1, wsReaders: 3 },
    })
  })

  it('reads levels, hold duration, and VUs per level', (): void => {
    deepEqual(
      parseRampPlan({
        RAMP_CHANGES_READERS_PER_LEVEL: '1',
        RAMP_LEVELS: '2,6',
        RAMP_STEP_DURATION: '1m30s',
        RAMP_WRITERS_PER_LEVEL: '2',
        RAMP_WS_READERS_PER_LEVEL: '4',
      }),
      {
        holdMs: 90_000,
        levels: [2, 6],
        transitionMs: 10_000,
        vusPerLevel: { changesReaders: 1, writers: 2, wsReaders: 4 },
      },
    )
  })

  it('rejects hold durations too short to measure a p95', (): void => {
    throws(
      () => parseRampPlan({ RAMP_STEP_DURATION: '4s' }),
      /RAMP_STEP_DURATION must be at least 5s/,
    )
  })
})

describe('parseRampCriteria', (): void => {
  it('uses the documented defaults', (): void => {
    deepEqual(parseRampCriteria({}), criteria)
  })

  it('reads explicit values', (): void => {
    deepEqual(
      parseRampCriteria({
        LATENCY_MULTIPLIER: '3',
        MIN_SUCCESS_RATE: '0.95',
        P95_LIMIT_MS: '500',
      }),
      { latencyMultiplier: 3, minimumSuccessRate: 0.95, p95LimitMs: 500 },
    )
  })

  it('rejects invalid values', (): void => {
    throws(() => parseRampCriteria({ MIN_SUCCESS_RATE: '0' }), /MIN_SUCCESS_RATE/)
    throws(() => parseRampCriteria({ MIN_SUCCESS_RATE: '1.5' }), /MIN_SUCCESS_RATE/)
    throws(() => parseRampCriteria({ P95_LIMIT_MS: '-1' }), /P95_LIMIT_MS/)
    throws(() => parseRampCriteria({ LATENCY_MULTIPLIER: 'x' }), /LATENCY_MULTIPLIER/)
  })
})

describe('ramp schedule', (): void => {
  it('adds a transition and a hold stage per level', (): void => {
    deepEqual(buildRampingVuStages(plan, 3), [
      { duration: '10000ms', target: 3 },
      { duration: '60000ms', target: 3 },
      { duration: '10000ms', target: 6 },
      { duration: '60000ms', target: 6 },
      { duration: '10000ms', target: 12 },
      { duration: '60000ms', target: 12 },
    ])
    equal(getRampTotalDurationMs(plan), 210_000)
  })

  it('maps elapsed time to the held level or a transition', (): void => {
    deepEqual(getRampPhaseAt(plan, 0), { endsAtMs: 10_000, level: null })
    deepEqual(getRampPhaseAt(plan, 10_000), { endsAtMs: 70_000, level: 1 })
    deepEqual(getRampPhaseAt(plan, 69_999), { endsAtMs: 70_000, level: 1 })
    deepEqual(getRampPhaseAt(plan, 75_000), { endsAtMs: 80_000, level: null })
    deepEqual(getRampPhaseAt(plan, 150_000), { endsAtMs: 210_000, level: 4 })
    deepEqual(getRampPhaseAt(plan, 250_000), { endsAtMs: 210_000, level: null })
  })
})

describe('buildRampThresholds', (): void => {
  it('declares a submetric per level for every judged metric', (): void => {
    const thresholds = buildRampThresholds({ ...plan, levels: [1] })

    deepEqual(Object.keys(thresholds).sort(), [
      'cleanup_success',
      'http_req_duration{operation:file-upload,level:1}',
      'http_req_failed',
      'shared_drive_changes_duration_ms{level:1}',
      'shared_drive_changes_success_rate{level:1}',
      'shared_drive_upload_success_rate{level:1}',
      'shared_drive_ws_auth_rate{level:1}',
      'shared_drive_ws_handshake_rate{level:1}',
    ])
    deepEqual(thresholds.http_req_failed, [
      { abortOnFail: true, delayAbortEval: '30s', threshold: 'rate<0.5' },
    ])
  })
})

describe('evaluateRampLevels', (): void => {
  it('reports a lower bound when every level passes', (): void => {
    const report = evaluateRampLevels(
      makeMetrics({ 1: healthy, 2: healthy, 4: healthy }),
      plan,
      criteria,
    )

    equal(report.max_sustained_level, 4)
    equal(report.result_bound, 'lower_bound')
    deepEqual(
      report.levels.map(({ status }) => status),
      ['passed', 'passed', 'passed'],
    )
    equal(report.criteria.upload_p95_limit_ms, 200)
    equal(report.criteria.changes_p95_limit_ms, 100)
  })

  it('stops at the first level whose success rate is too low', (): void => {
    const report = evaluateRampLevels(
      makeMetrics({ 1: healthy, 2: { ...healthy, wsAuth: 0.5 }, 4: healthy }),
      plan,
      criteria,
    )

    equal(report.max_sustained_level, 1)
    equal(report.result_bound, 'exact')
    deepEqual(report.levels[1]?.reasons, [
      'ws auth rate 0.5 is below 0.99',
    ])
  })

  it('judges latency against a multiple of the first level', (): void => {
    const report = evaluateRampLevels(
      makeMetrics({
        1: healthy,
        2: { ...healthy, uploadP95: 201 },
        4: healthy,
      }),
      plan,
      criteria,
    )

    equal(report.max_sustained_level, 1)
    deepEqual(report.levels[1]?.reasons, [
      'upload p95 201 ms exceeds 200 ms',
    ])
  })

  it('uses an absolute p95 limit when configured', (): void => {
    const report = evaluateRampLevels(
      makeMetrics({ 1: { ...healthy, uploadP95: 400 } }),
      { ...plan, levels: [1] },
      { ...criteria, p95LimitMs: 300 },
    )

    equal(report.levels[0]?.status, 'failed')
    equal(report.criteria.upload_p95_limit_ms, 300)
  })

  it('never counts a level with missing metrics as passed', (): void => {
    const report = evaluateRampLevels(
      makeMetrics({ 1: healthy, 2: { ...healthy, wsAuth: undefined } }),
      plan,
      criteria,
    )

    equal(report.levels[1]?.status, 'not_measured')
    deepEqual(report.levels[1]?.reasons, ['ws auth rate has no samples'])
    equal(report.max_sustained_level, 1)
    equal(report.result_bound, 'inconclusive')
  })

  it('treats a p95 without samples as missing, not as zero', (): void => {
    const metrics = makeMetrics({ 1: healthy, 2: healthy })
    metrics[
      rampLevelMetricName('shared_drive_changes_duration_ms', 2)
    ] = { values: { 'p(95)': 0 } }

    const report = evaluateRampLevels(metrics, plan, criteria)

    equal(report.levels[1]?.status, 'not_measured')
    deepEqual(report.levels[1]?.reasons, ['changes p95 has no samples'])
  })

  it('marks levels the run never reached', (): void => {
    const report = evaluateRampLevels(makeMetrics({ 1: healthy }), plan, criteria)

    deepEqual(
      report.levels.map(({ status }) => status),
      ['passed', 'not_run', 'not_run'],
    )
    equal(report.result_bound, 'inconclusive')
  })

  it('cannot judge latency without a first-level baseline', (): void => {
    const report = evaluateRampLevels(
      makeMetrics({ 1: { ...healthy, uploadP95: undefined } }),
      { ...plan, levels: [1] },
      criteria,
    )

    equal(report.levels[0]?.status, 'not_measured')
    equal(report.max_sustained_level, 0)
  })
})

describe('formatRampReport', (): void => {
  it('prints one line per level and the result', (): void => {
    const text = formatRampReport(
      evaluateRampLevels(
        makeMetrics({ 1: healthy, 2: { ...healthy, uploadSuccess: 0.9 } }),
        plan,
        criteria,
      ),
    )

    match(text, /level 1: passed/)
    match(text, /level 2: failed \(upload success rate 0\.9 is below 0\.99\)/)
    match(text, /level 4: not_run/)
    match(text, /Maximum sustained level: 1 \(exact\)/)
  })
})
