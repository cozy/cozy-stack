import assert from 'node:assert/strict'
import test from 'node:test'

import { parsePositiveIntegerEnv } from './env.ts'

test('parsePositiveIntegerEnv handles missing, default, and valid integer', () => {
  assert.equal(parsePositiveIntegerEnv({}, 'VUS', 3), 3)
  assert.equal(parsePositiveIntegerEnv({ VUS: '' }, 'VUS', 3), 3)
  assert.equal(parsePositiveIntegerEnv({ VUS: '  ' }, 'VUS', 3), 3)
  assert.equal(parsePositiveIntegerEnv({ VUS: '5' }, 'VUS', 3), 5)
  assert.equal(parsePositiveIntegerEnv({ VUS: ' 10 ' }, 'VUS', 3), 10)
})

test('parsePositiveIntegerEnv throws on non-integer, negative, or zero', () => {
  assert.throws(() => parsePositiveIntegerEnv({ VUS: '0' }, 'VUS', 3), /must be a positive integer/)
  assert.throws(() => parsePositiveIntegerEnv({ VUS: '-1' }, 'VUS', 3), /must be a positive integer/)
  assert.throws(() => parsePositiveIntegerEnv({ VUS: '1.5' }, 'VUS', 3), /must be a positive integer/)
  assert.throws(() => parsePositiveIntegerEnv({ VUS: 'abc' }, 'VUS', 3), /must be a positive integer/)
})
