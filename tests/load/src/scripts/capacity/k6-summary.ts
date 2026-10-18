export type JsonRecord = Record<string, unknown>

export function isJsonRecord(value: unknown): value is JsonRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

// Reads one field from a `k6 run --summary-export` file. The first metric
// name that holds a finite number wins, so callers list submetrics first.
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
