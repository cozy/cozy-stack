import assert from 'node:assert/strict'
import test from 'node:test'

import {
  formatAuthMessage,
  getSharedDriveWsUrl,
  parseRealtimeMessage,
} from './cozy-realtime'

test('formatAuthMessage formats valid JSON', () => {
  const msg = formatAuthMessage('secret-token-123')
  assert.deepEqual(JSON.parse(msg), {
    method: 'AUTH',
    payload: 'secret-token-123',
  })
})

test('parseRealtimeMessage handles CREATED, UPDATED and error events', () => {
  const created = parseRealtimeMessage(
    '{"event":"CREATED","payload":{"type":"io.cozy.files","id":"file-999"}}',
  )
  assert.deepEqual(created, {
    event: 'CREATED',
    payload: {
      type: 'io.cozy.files',
      id: 'file-999',
    },
  })

  const errorMsg = parseRealtimeMessage(
    '{"event":"error","payload":{"status":"401 Unauthorized","code":"unauthorized","title":"The authentication has failed"}}',
  )
  assert.deepEqual(errorMsg, {
    event: 'error',
    payload: {
      status: '401 Unauthorized',
      code: 'unauthorized',
      title: 'The authentication has failed',
    },
  })
})

test('parseRealtimeMessage rejects invalid or non-JSON payloads', () => {
  assert.equal(parseRealtimeMessage('invalid json'), null)
  assert.equal(parseRealtimeMessage('123'), null)
  assert.equal(parseRealtimeMessage('{"other":"value"}'), null)
})

test('getSharedDriveWsUrl converts HTTP to WS and HTTPS to WSS', () => {
  assert.equal(
    getSharedDriveWsUrl('http://load.localhost:8080', 'drive-123'),
    'ws://load.localhost:8080/sharings/drives/drive-123/realtime',
  )
  assert.equal(
    getSharedDriveWsUrl('https://my-cozy.example.com/', 'drive-abc'),
    'wss://my-cozy.example.com/sharings/drives/drive-abc/realtime',
  )
})
