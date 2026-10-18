import { getMetricNumber, isJsonRecord } from './k6-summary.ts'
import type { PointResult } from './search.ts'

export type UploadPointMetrics = {
  cleanupRate: number
  completedIterations: number
  // Null when the scenario predates the upload_consistency metric.
  consistencyRate: number | null
  droppedIterations: number
  p95Ms: number
  successRate: number
}

export type UploadPointClassificationInput = {
  expectedIterations: number
  metrics: UploadPointMetrics | null
  minimumSuccessRate: number
  p95LimitMs: number | null
  runnerExitCode: number
  summaryFound: boolean
}

export type UploadPointClassification = {
  reason: string
  result: PointResult
}

export function readUploadPointMetrics(
  summary: unknown,
): UploadPointMetrics | null {
  if (!isJsonRecord(summary)) {
    return null
  }

  const p95Ms = getMetricNumber(
    summary,
    ['http_req_duration{operation:file-upload}', 'http_req_duration'],
    'p(95)',
  )
  const successRate = getMetricNumber(
    summary,
    ['upload_success{operation:file-upload}', 'upload_success'],
    'value',
  )
  const completedIterations = getMetricNumber(summary, ['iterations'], 'count')
  const cleanupRate = getMetricNumber(summary, ['cleanup_success'], 'value')
  const consistencyRate = getMetricNumber(
    summary,
    ['upload_consistency'],
    'value',
  )
  const droppedIterations =
    getMetricNumber(summary, ['dropped_iterations'], 'count') ?? 0

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
    consistencyRate,
    droppedIterations,
    p95Ms,
    successRate,
  }
}

export function isInterruptedCapacityPoint(
  runnerExitCode: number,
  completedIterations: number,
  expectedIterations: number,
): boolean {
  return runnerExitCode !== 0 && completedIterations !== expectedIterations
}

function getMeasuredFailure(
  metrics: UploadPointMetrics,
  input: UploadPointClassificationInput,
): string {
  if (
    metrics.completedIterations !== input.expectedIterations ||
    metrics.droppedIterations !== 0
  ) {
    return 'iterations were dropped or incomplete'
  }
  if (metrics.successRate < input.minimumSuccessRate) {
    return 'upload success rate is below the minimum'
  }
  if (input.p95LimitMs !== null && metrics.p95Ms > input.p95LimitMs) {
    return 'upload p95 exceeds the configured limit'
  }

  return ''
}

// A content mismatch fails the point first: k6 aborts on it, which would
// otherwise look like an interrupted run. Other infrastructure problems come
// next, because a point that did not run completely or cleanly must not count
// as a capacity measurement.
export function classifyUploadPoint(
  input: UploadPointClassificationInput,
): UploadPointClassification {
  const infrastructureFailure = (reason: string): UploadPointClassification => ({
    reason,
    result: 'infrastructure_failure',
  })
  const { metrics } = input

  if (!input.summaryFound) {
    return infrastructureFailure('k6 produced no summary')
  }
  if (metrics === null) {
    return infrastructureFailure('summary is missing required metrics')
  }
  if (metrics.consistencyRate !== null && metrics.consistencyRate < 1) {
    return {
      reason: 'uploaded content did not match after download',
      result: 'failed',
    }
  }
  if (metrics.cleanupRate !== 1) {
    return infrastructureFailure('test-directory cleanup failed')
  }
  if (
    isInterruptedCapacityPoint(
      input.runnerExitCode,
      metrics.completedIterations,
      input.expectedIterations,
    )
  ) {
    return infrastructureFailure(
      'k6 stopped before completing every upload consistency check',
    )
  }
  if (metrics.completedIterations === 0) {
    return infrastructureFailure('k6 completed no upload iterations')
  }

  const measuredFailure = getMeasuredFailure(metrics, input)

  if (measuredFailure.length > 0) {
    return { reason: measuredFailure, result: 'failed' }
  }
  if (input.runnerExitCode !== 0) {
    return infrastructureFailure(
      'k6 exited unsuccessfully even though measured criteria passed',
    )
  }

  return { reason: '', result: 'passed' }
}
