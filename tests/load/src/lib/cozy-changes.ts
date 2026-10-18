export interface ChangesFeedItem {
  seq: string
  id: string
  deleted?: boolean
  changes?: Array<{ rev: string }>
  doc?: Record<string, unknown>
}

export interface ChangesFeedResponse {
  results: ChangesFeedItem[]
  last_seq: string
  pending: number
}

export interface BuildChangesFeedUrlParams {
  baseUrl: string
  sharingId: string
  since?: string
  limit?: number
  includeDocs?: boolean
}

export function buildChangesFeedUrl({
  baseUrl,
  sharingId,
  since,
  limit,
  includeDocs,
}: BuildChangesFeedUrlParams): string {
  const trimmedBase = baseUrl.trim().replace(/\/+$/, '')
  const query: string[] = []

  if (since !== undefined) {
    query.push(`since=${encodeURIComponent(since)}`)
  }
  if (limit !== undefined) {
    query.push(`limit=${encodeURIComponent(String(limit))}`)
  }
  if (includeDocs !== undefined) {
    query.push(`include_docs=${encodeURIComponent(String(includeDocs))}`)
  }

  const queryString = query.length > 0 ? `?${query.join('&')}` : ''
  return `${trimmedBase}/sharings/drives/${encodeURIComponent(sharingId)}/_changes${queryString}`
}

export function parseChangesFeedResponse(rawBody: string): ChangesFeedResponse {
  if (!rawBody || typeof rawBody !== 'string') {
    throw new Error('Changes feed response body must be a non-empty string')
  }

  let parsed: unknown
  try {
    parsed = JSON.parse(rawBody)
  } catch (error) {
    throw new Error(`Failed to parse changes feed response as JSON: ${String(error)}`)
  }

  if (!parsed || typeof parsed !== 'object') {
    throw new Error('Changes feed response must be a JSON object')
  }

  const candidate = parsed as Record<string, unknown>
  const results = Array.isArray(candidate.results)
    ? (candidate.results as ChangesFeedItem[])
    : []
  const lastSeq =
    typeof candidate.last_seq === 'string'
      ? candidate.last_seq
      : String(candidate.last_seq ?? '0')
  const pending =
    typeof candidate.pending === 'number' ? candidate.pending : 0

  return {
    results,
    last_seq: lastSeq,
    pending,
  }
}
