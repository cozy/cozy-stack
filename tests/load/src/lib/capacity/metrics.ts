import type { ResultBound } from './types.ts'

type JsonRecord = Record<string, unknown>

export function isJsonRecord(value: unknown): value is JsonRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function getMetricNumber(
  summary: JsonRecord,
  metricNames: readonly string[],
  fieldName: string,
): number | null {
  const metrics = summary.metrics

  if (!isJsonRecord(metrics)) {
    return null
  }

  for (const metricName of metricNames) {
    const metric = metrics[metricName]

    if (!isJsonRecord(metric)) {
      continue
    }

    const value = metric[fieldName]

    if (typeof value === 'number' && Number.isFinite(value)) {
      return value
    }
  }

  return null
}

export function computeP95LimitMs(
  baselineP95Ms: number,
  latencyMultiplier: number,
  configuredP95LimitMs: number | null,
): number {
  return configuredP95LimitMs ?? baselineP95Ms * latencyMultiplier
}

export function getCapacityResultBound(
  candidateLevel: number,
  configuredMaxLevel: number,
  firstFailingLevel: number,
): ResultBound {
  return candidateLevel === configuredMaxLevel && firstFailingLevel === 0
    ? 'lower_bound'
    : 'exact'
}

export function isInterruptedCapacityPoint(
  runnerExitCode: number,
  completedIterations: number,
  expectedIterations: number,
): boolean {
  return runnerExitCode !== 0 && completedIterations !== expectedIterations
}
