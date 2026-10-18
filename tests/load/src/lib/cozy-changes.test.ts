import assert from 'node:assert/strict'
import test from 'node:test'

import {
  buildChangesFeedUrl,
  parseChangesFeedResponse,
} from './cozy-changes'

test('buildChangesFeedUrl builds valid URL with query parameters', () => {
  const url = buildChangesFeedUrl({
    baseUrl: 'http://load.localhost:8080',
    sharingId: 'drive-xyz-123',
    since: 'seq-1',
    limit: 50,
    includeDocs: true,
  })
  assert.equal(
    url,
    'http://load.localhost:8080/sharings/drives/drive-xyz-123/_changes?since=seq-1&limit=50&include_docs=true',
  )
})

test('buildChangesFeedUrl handles omitted optional parameters', () => {
  const url = buildChangesFeedUrl({
    baseUrl: 'http://load.localhost:8080/',
    sharingId: 'drive-xyz-123',
  })
  assert.equal(
    url,
    'http://load.localhost:8080/sharings/drives/drive-xyz-123/_changes',
  )
})

test('parseChangesFeedResponse parses valid changes feed output', () => {
  const json = JSON.stringify({
    results: [
      {
        seq: '1-abc',
        id: 'file-1',
        changes: [{ rev: '1-xxx' }],
        deleted: false,
      },
    ],
    last_seq: '1-abc',
    pending: 0,
  })

  const parsed = parseChangesFeedResponse(json)
  assert.equal(parsed.last_seq, '1-abc')
  assert.equal(parsed.pending, 0)
  assert.equal(parsed.results.length, 1)
  assert.equal(parsed.results[0]?.id, 'file-1')
})

test('parseChangesFeedResponse rejects invalid JSON or malformed input', () => {
  assert.throws(() => parseChangesFeedResponse(''), /non-empty string/)
  assert.throws(() => parseChangesFeedResponse('not json'), /Failed to parse/)
  assert.throws(() => parseChangesFeedResponse('null'), /JSON object/)
})
