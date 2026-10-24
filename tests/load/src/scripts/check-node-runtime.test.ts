import { equal, throws } from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

import { minimumNode, satisfiesMinimum } from './check-node-runtime.ts'

test('versions compare numerically against the engines minimum', () => {
  for (const [version, ok] of [
    ['v22.23.4', true], ['v22.23.10', true], ['v22.24.0', true], ['v23.0.0', true],
    ['v22.23.3', false], ['v22.23.2', false], ['v22.9.99', false], ['v21.99.99', false],
  ] as const) equal(satisfiesMinimum(version, '22.23.4'), ok, version)
})

test('only an exact >=X.Y.Z engines range is accepted', () => {
  const manifest = JSON.parse(readFileSync(new URL('../../package.json', import.meta.url), 'utf8'))
  equal(minimumNode(manifest.engines.node), '22.23.4')
  for (const range of ['>=22', '^22.23.4', '22.23.4', '>=22.23.4 <23']) throws(() => minimumNode(range), /Unsupported/)
})
