import { fileURLToPath } from 'node:url'
import { resolve } from 'node:path'

import {
  computeP95LimitMs,
  getCapacityResultBound,
  isInterruptedCapacityPoint,
  type ResultBound,
} from '../lib/capacity/index.ts'
import { runCapacityCampaign } from './find-capacity.ts'

export {
  computeP95LimitMs,
  getCapacityResultBound,
  isInterruptedCapacityPoint,
  type ResultBound,
}

function isMainModule(): boolean {
  const entryPath = process.argv[1]
  return (
    entryPath !== undefined &&
    resolve(entryPath) === fileURLToPath(import.meta.url)
  )
}

if (isMainModule()) {
  process.env.SCENARIO = 'upload'
  process.exitCode = runCapacityCampaign()
}
