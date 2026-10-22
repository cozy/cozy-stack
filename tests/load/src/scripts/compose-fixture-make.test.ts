import { equal, match } from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { it } from 'node:test'

const cwd = fileURLToPath(new URL('../../', import.meta.url))

for (const operation of ['up', 'snapshot', 'restore', 'discard']) {
  it(`Make exposes fixture-${operation} with quoted environment arguments`, (): void => {
    const result = spawnSync('make', ['--no-print-directory', '-n', '-o', 'node_modules/.load-ready',
      `fixture-${operation}`], {
      cwd, encoding: 'utf8',
      env: { ...process.env, FIXTURE_PROJECT: 'load-fixture-test',
        FIXTURE_SNAPSHOT: '/private/baseline with spaces' },
    })
    equal(result.status, 0, result.stderr)
    match(result.stdout, new RegExp(`compose-fixture\\.ts ${operation} "\\$FIXTURE_PROJECT"`))
    if (operation === 'snapshot' || operation === 'restore') {
      match(result.stdout, /"\$FIXTURE_SNAPSHOT"/)
    }
  })
}
