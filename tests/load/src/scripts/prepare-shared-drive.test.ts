import { equal, ok, rejects } from 'node:assert/strict'
import { existsSync, readFileSync, unlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, it } from 'node:test'

import {
  createSharedDrive,
  ensureSharedDrive,
  updateEnvFileWithSharedDrive,
  type APISharingItem,
  type SharedDriveInfo,
} from './prepare-shared-drive.ts'

describe('updateEnvFileWithSharedDrive', (): void => {
  it('writes only SHARED_DRIVE_ID and SHARED_DRIVE_ROOT_ID without touching UPLOAD_TARGET', (): void => {
    const tempPath = join(
      tmpdir(),
      `test-sd-env-${Date.now()}-${Math.random().toString(36).slice(2)}.env`,
    )
    try {
      writeFileSync(tempPath, 'FOO=bar\nUPLOAD_TARGET=personal\n', 'utf8')

      const drive: SharedDriveInfo = {
        sharingId: 'sharing-123',
        rootDirectoryId: 'dir-456',
        description: 'Test Drive',
      }

      updateEnvFileWithSharedDrive(tempPath, drive)

      const content = readFileSync(tempPath, 'utf8')
      ok(content.includes('FOO=bar'))
      ok(content.includes('UPLOAD_TARGET=personal'))
      ok(!content.includes('UPLOAD_TARGET=shared-drive'))
      ok(content.includes('SHARED_DRIVE_ID=sharing-123'))
      ok(content.includes('SHARED_DRIVE_ROOT_ID=dir-456'))
    } finally {
      if (existsSync(tempPath)) {
        unlinkSync(tempPath)
      }
    }
  })
})

describe('ensureSharedDrive', (): void => {
  it('filters out inactive or non-owner or file-root drives', async (): Promise<void> => {
    const mockDrives: APISharingItem[] = [
      {
        id: 'inactive-drive',
        attributes: {
          description: 'Load-Test Shared Drive',
          active: false,
          owner: true,
          drive_root_type: 'directory',
          rules: [{ values: ['dir-inactive'] }],
        },
      },
      {
        id: 'recipient-drive',
        attributes: {
          description: 'Load-Test Shared Drive',
          active: true,
          owner: false,
          drive_root_type: 'directory',
          rules: [{ values: ['dir-recipient'] }],
        },
      },
      {
        id: 'file-drive',
        attributes: {
          description: 'Load-Test Shared Drive',
          active: true,
          owner: true,
          drive_root_type: 'file',
          rules: [{ values: ['file-123'] }],
        },
      },
      {
        id: 'valid-drive',
        attributes: {
          description: 'Load-Test Shared Drive',
          active: true,
          owner: true,
          drive_root_type: 'directory',
          rules: [{ values: ['dir-valid-root'] }],
        },
      },
    ]

    const originalFetch = globalThis.fetch
    try {
      globalThis.fetch = async (): Promise<Response> => {
        return new Response(JSON.stringify({ data: mockDrives }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }

      const result = await ensureSharedDrive({
        baseUrl: 'http://load.localhost:8080',
        accessToken: 'mock-token',
        driveName: 'Load-Test Shared Drive',
      })

      equal(result.sharingId, 'valid-drive')
      equal(result.rootDirectoryId, 'dir-valid-root')
    } finally {
      globalThis.fetch = originalFetch
    }
  })

  it('throws if created drive has an empty rootDirectoryId', async (): Promise<void> => {
    const originalFetch = globalThis.fetch
    try {
      globalThis.fetch = async (): Promise<Response> => {
        return new Response(
          JSON.stringify({
            data: {
              id: 'new-drive-id',
              attributes: {
                description: 'Load-Test Shared Drive',
                rules: [{ values: [] }],
              },
            },
          }),
          {
            status: 201,
            headers: { 'Content-Type': 'application/json' },
          },
        )
      }

      await rejects(
        createSharedDrive({
          baseUrl: 'http://load.localhost:8080',
          accessToken: 'mock-token',
          driveName: 'Load-Test Shared Drive',
        }),
        /Created shared drive new-drive-id has no root directory ID/,
      )
    } finally {
      globalThis.fetch = originalFetch
    }
  })
})
