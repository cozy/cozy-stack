import { deepEqual, equal, throws } from 'node:assert/strict'
import { existsSync, readFileSync, statSync, unlinkSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, it } from 'node:test'

import {
  getUpdatedEnvironmentLines,
  saveEnvironmentFile,
} from './env.ts'

describe('getUpdatedEnvironmentLines', (): void => {
  it('updates existing keys while preserving comments and order', (): void => {
    const initialLines = [
      '# Header comment',
      'BASE_URL=http://localhost:8080',
      '',
      'COZY_ACCESS_TOKEN=old-token',
      '# Footer comment',
    ]
    const updates = {
      COZY_ACCESS_TOKEN: 'new-token',
    }
    const result = getUpdatedEnvironmentLines(initialLines, updates)
    deepEqual(result, [
      '# Header comment',
      'BASE_URL=http://localhost:8080',
      '',
      'COZY_ACCESS_TOKEN=new-token',
      '# Footer comment',
    ])
  })

  it('updates every copy of a duplicated key', (): void => {
    const initialLines = [
      'COZY_ACCESS_TOKEN=old1',
      'BASE_URL=http://localhost:8080',
      'COZY_ACCESS_TOKEN=old2',
    ]
    const updates = {
      COZY_ACCESS_TOKEN: 'new-token',
    }
    const result = getUpdatedEnvironmentLines(initialLines, updates)
    deepEqual(result, [
      'COZY_ACCESS_TOKEN=new-token',
      'BASE_URL=http://localhost:8080',
      'COZY_ACCESS_TOKEN=new-token',
    ])
  })

  it('appends new keys at the end', (): void => {
    const initialLines = ['BASE_URL=http://localhost:8080']
    const updates = {
      SHARED_DRIVE_ID: 'drive-123',
    }
    const result = getUpdatedEnvironmentLines(initialLines, updates)
    deepEqual(result, [
      'BASE_URL=http://localhost:8080',
      'SHARED_DRIVE_ID=drive-123',
    ])
  })
})

describe('saveEnvironmentFile', (): void => {
  it('writes atomically with 0600 permissions and trailing newline', (): void => {
    const tempPath = join(tmpdir(), `test-env-${Date.now()}-${Math.random().toString(36).slice(2)}.env`)
    try {
      saveEnvironmentFile(tempPath, {
        SHARED_DRIVE_ID: 'drive-abc',
        SHARED_DRIVE_ROOT_ID: 'dir-xyz',
      })

      equal(existsSync(tempPath), true)
      const content = readFileSync(tempPath, 'utf8')
      equal(content, 'SHARED_DRIVE_ID=drive-abc\nSHARED_DRIVE_ROOT_ID=dir-xyz\n')

      const stats = statSync(tempPath)
      equal(stats.mode & 0o777, 0o600)
    } finally {
      if (existsSync(tempPath)) {
        unlinkSync(tempPath)
      }
    }
  })

  it('rejects multiline values', (): void => {
    const tempPath = join(tmpdir(), `test-env-${Date.now()}.env`)
    throws(
      (): void => {
        saveEnvironmentFile(tempPath, {
          BAD: 'val\ninvalid',
        })
      },
      /Environment values must stay on one line/,
    )
  })
})
