import { check, sleep } from 'k6'
import { md5 } from 'k6/crypto'
import exec from 'k6/execution'
import http from 'k6/http'
import { Counter, Rate, Trend } from 'k6/metrics'
import type { Options } from 'k6/options'
import ws from 'k6/ws'

import { randomizeUploadData } from '../fixtures/upload-data.ts'
import {
  buildChangesFeedUrl,
  parseChangesFeedResponse,
} from '../lib/cozy-changes.ts'
import {
  fetchCreatedTestDirectory,
  fetchDestroyResponse,
  fetchTrashResponse,
  fetchUploadResponse,
  type TestDirectory,
} from '../lib/cozy-files.ts'
import {
  formatAuthMessage,
  getSharedDriveWsUrl,
  parseRealtimeMessage,
} from '../lib/cozy-realtime.ts'
import {
  getWsSessionDurationMs,
  parseDurationToMs,
} from '../lib/duration.ts'
import { parsePositiveIntegerEnv } from '../lib/env.ts'
import {
  getUploadTarget,
  type SharedDriveUploadTarget,
} from '../lib/upload-target.ts'

// --- Metrics ---
export const cleanupSuccess = new Rate('cleanup_success')

export const uploadSuccessRate = new Rate('shared_drive_upload_success_rate')

export const wsHandshakeSuccessRate = new Rate('shared_drive_ws_handshake_rate')
export const wsAuthSuccessRate = new Rate('shared_drive_ws_auth_rate')
export const wsMessagesReceived = new Counter('shared_drive_ws_messages_total')
export const wsEventsReceived = new Counter('shared_drive_ws_events_total')
export const wsErrorsReceived = new Counter('shared_drive_ws_errors_total')

export const changesFeedDuration = new Trend('shared_drive_changes_duration_ms', true)
export const changesFeedSuccessRate = new Rate('shared_drive_changes_success_rate')
export const changesItemsReceived = new Counter('shared_drive_changes_items_total')
export const changesPending = new Trend('shared_drive_changes_pending')

// --- Configuration ---
const rawDuration = __ENV.DURATION || '30s'
const rawWritersVus = parsePositiveIntegerEnv(__ENV, 'WRITER_VUS', 3)
const rawWsReadersVus = parsePositiveIntegerEnv(__ENV, 'WS_READER_VUS', 5)
const rawChangesReadersVus = parsePositiveIntegerEnv(__ENV, 'CHANGES_READER_VUS', 3)

export const options: Options = {
  scenarios: {
    writers: {
      executor: 'constant-vus',
      vus: rawWritersVus,
      duration: rawDuration,
      exec: 'writerLoop',
    },
    ws_readers: {
      executor: 'constant-vus',
      vus: rawWsReadersVus,
      duration: rawDuration,
      exec: 'wsReaderLoop',
    },
    changes_readers: {
      executor: 'constant-vus',
      vus: rawChangesReadersVus,
      duration: rawDuration,
      exec: 'changesReaderLoop',
    },
  },
  thresholds: {
    cleanup_success: ['rate==1'],
    shared_drive_upload_success_rate: ['rate>0.95'],
    shared_drive_ws_handshake_rate: ['rate>0.95'],
    shared_drive_ws_auth_rate: ['rate>0.95'],
    shared_drive_changes_success_rate: ['rate>0.95'],
    shared_drive_ws_events_total: ['count>0'],
    shared_drive_changes_items_total: ['count>0'],
  },
}

interface SetupData {
  accessToken: string
  baseUrl: string
  driveId: string
  rootDirectoryId: string
  testDir: TestDirectory
  testDurationMs: number
  wsSessionDurationMs: number
}

export function setup(): SetupData {
  const accessToken = __ENV.COZY_ACCESS_TOKEN
  if (!accessToken) {
    throw new Error('COZY_ACCESS_TOKEN environment variable is required')
  }

  const baseUrl = __ENV.COZY_BASE_URL || __ENV.BASE_URL || 'http://localhost:8080'
  const uploadTarget = getUploadTarget(__ENV)

  if (uploadTarget.kind !== 'shared-drive') {
    throw new Error('Expected shared-drive upload target')
  }

  const directoryName = `shared-drive-test-${Date.now()}`
  const testDir = fetchCreatedTestDirectory({
    accessToken,
    baseUrl,
    directoryName,
    tags: { suite: 'shared-drive-load' },
    target: uploadTarget,
    timeout: '30s',
  })

  const testDurationMs = parseDurationToMs(rawDuration)
  const wsSessionDurationMs = getWsSessionDurationMs(
    rawDuration,
    __ENV.WS_SESSION_DURATION,
  )

  return {
    accessToken,
    baseUrl,
    driveId: uploadTarget.driveId,
    rootDirectoryId: uploadTarget.rootDirectoryId,
    testDir,
    testDurationMs,
    wsSessionDurationMs,
  }
}

// --- Scenario 1: Writers ---
export function writerLoop(data: SetupData): void {
  const vu = exec.vu.idInTest
  const iter = exec.vu.iterationInInstance
  const marker = [iter, vu, Date.now(), Math.floor(Math.random() * 0xffffffff)].join('-')
  const filename = `sd-vu${vu}-iter${iter}-${marker}.bin`
  const uploadBody = randomizeUploadData(marker)
  const expectedChecksum = md5(uploadBody, 'base64')

  const target: SharedDriveUploadTarget = {
    kind: 'shared-drive',
    driveId: data.driveId,
    rootDirectoryId: data.testDir.id,
  }

  const res = fetchUploadResponse({
    accessToken: data.accessToken,
    baseUrl: data.baseUrl,
    body: uploadBody,
    contentMD5: expectedChecksum,
    directoryId: data.testDir.id,
    filename,
    tags: { scenario: 'writers' },
    target,
    timeout: '30s',
  })

  const success = check(res, {
    'writer uploaded file successfully': (r) => r.status === 201,
  })
  uploadSuccessRate.add(success ? 1 : 0)

  sleep(1)
}

// --- Scenario 2: WebSocket Readers ---
export function wsReaderLoop(data: SetupData): void {
  const scenarioEndTime = exec.scenario.startTime + data.testDurationMs
  const timeRemainingMs = scenarioEndTime - Date.now()
  if (timeRemainingMs < 1000) {
    sleep(1)
    return
  }

  const sessionDurationMs = Math.min(data.wsSessionDurationMs, timeRemainingMs)
  const wsUrl = getSharedDriveWsUrl(data.baseUrl, data.driveId)
  const params = {
    tags: { scenario: 'ws_readers' },
  }

  let hadAuthError = false

  const res = ws.connect(wsUrl, params, (socket) => {
    socket.on('open', () => {
      socket.send(formatAuthMessage(data.accessToken))
    })

    socket.on('message', (raw: string) => {
      wsMessagesReceived.add(1)
      const parsed = parseRealtimeMessage(raw)
      if (parsed) {
        if (parsed.event === 'error') {
          hadAuthError = true
          wsErrorsReceived.add(1)
          wsAuthSuccessRate.add(0)
        } else if (parsed.event === 'CREATED' || parsed.event === 'UPDATED') {
          wsEventsReceived.add(1, { event: parsed.event })
          check(parsed, {
            'ws received valid file event': (p) =>
              Boolean(p.payload && p.payload.type === 'io.cozy.files' && p.payload.id),
          })
        }
      }
    })

    socket.on('close', () => {
      if (!hadAuthError) {
        wsAuthSuccessRate.add(1)
      }
    })

    socket.setTimeout(() => {
      socket.close()
    }, sessionDurationMs)
  })

  const handshakeOk = check(res, {
    'ws handshake switching protocols': (r) => Boolean(r && r.status === 101),
  })
  wsHandshakeSuccessRate.add(handshakeOk ? 1 : 0)

  check(hadAuthError, {
    'ws session completed without auth error': (err) => !err,
  })

  sleep(1)
}

// --- Scenario 3: Changes Feed Readers ---
let vuSince = 'now'

export function changesReaderLoop(data: SetupData): void {
  const headers = {
    Authorization: `Bearer ${data.accessToken}`,
    Accept: 'application/json',
  }

  const url = buildChangesFeedUrl({
    baseUrl: data.baseUrl,
    sharingId: data.driveId,
    since: vuSince,
    limit: 100,
    includeDocs: false,
  })

  const startTime = Date.now()
  const res = http.get(url, { headers, tags: { scenario: 'changes_readers' } })
  const duration = Date.now() - startTime
  changesFeedDuration.add(duration)

  const ok = check(res, {
    'changes feed returned 200': (r) => r.status === 200,
  })
  changesFeedSuccessRate.add(ok ? 1 : 0)

  if (ok && typeof res.body === 'string') {
    try {
      const parsed = parseChangesFeedResponse(res.body)
      vuSince = parsed.last_seq
      if (parsed.results.length > 0) {
        changesItemsReceived.add(parsed.results.length)
      }
      changesPending.add(parsed.pending)
      if (parsed.pending > 0) {
        return
      }
    } catch {
      // Ignore parse error on truncated body
    }
  }

  sleep(1)
}

// --- Teardown ---
export function teardown(data: SetupData): void {
  if (!data?.testDir?.id) {
    cleanupSuccess.add(false, { scenario: 'teardown' })
    return
  }

  const target: SharedDriveUploadTarget = {
    kind: 'shared-drive',
    driveId: data.driveId,
    rootDirectoryId: data.rootDirectoryId,
  }

  const requestOptions = {
    accessToken: data.accessToken,
    baseUrl: data.baseUrl,
    directoryId: data.testDir.id,
    tags: { scenario: 'teardown' },
    target,
    timeout: '30s',
  }

  let ok = true
  const trashRes = fetchTrashResponse(requestOptions)
  if (trashRes.status !== 200) {
    ok = false
  }

  const destroyRes = fetchDestroyResponse(requestOptions)
  if (destroyRes.status !== 204) {
    ok = false
  }

  cleanupSuccess.add(ok, { scenario: 'teardown' })
}
