import { equal } from 'node:assert/strict'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, it } from 'node:test'

import {
  makeCampaignState,
  runCapacitySearch,
  type BaseCapacityConfig,
} from './engine.ts'
import type { CapacityPhase, CapacityScenario, PointRunResult } from './types.ts'

describe('CapacityEngine', (): void => {
  function makeTestDir(): string {
    return mkdtempSync(join(tmpdir(), 'capacity-engine-test-'))
  }

  it('finds maximum capacity when all levels pass up to maxLevel', (): void => {
    const testDir = makeTestDir()
    try {
      const mockScenario: CapacityScenario<BaseCapacityConfig> = {
        name: 'mock',
        unit: 'workers',
        runPoint(
          _phase: CapacityPhase,
          _level: number,
          _iterations: number,
          _p95Limit: number | null,
          _config: BaseCapacityConfig,
          _attempt: number,
          executionId: string,
        ): PointRunResult {
          return {
            result: 'passed',
            p95Ms: 100,
            successRate: 1,
            droppedIterations: 0,
            completedIterations: 10,
            cleanupRate: 1,
            runnerExitCode: 0,
            summaryPath: `${executionId}-summary.json`,
            reason: '',
          }
        },
      }

      const config: BaseCapacityConfig = {
        campaignId: 'test-all-pass',
        targetUrl: 'http://localhost',
        maxLevel: 8,
        baselineIterations: 5,
        confirmationIterations: 10,
        latencyMultiplier: 2,
        configuredP95LimitMs: null,
        minimumSuccessRate: 0.95,
      }

      const state = makeCampaignState(mockScenario, config, {
        resultsDirectory: testDir,
      })

      const exitCode = runCapacitySearch(state)
      equal(exitCode, 0)
      equal(state.report.status, 'completed')
      equal(state.report.maximum_capacity, 8)
      equal(state.report.result_bound, 'lower_bound')

      const reportOnDisk = JSON.parse(readFileSync(state.reportPath, 'utf8'))
      equal(reportOnDisk.status, 'completed')
      equal(reportOnDisk.maximum_capacity, 8)
    } finally {
      rmSync(testDir, { recursive: true, force: true })
    }
  })

  it('stops and reports 0 when baseline fails', (): void => {
    const testDir = makeTestDir()
    try {
      const mockScenario: CapacityScenario<BaseCapacityConfig> = {
        name: 'mock',
        unit: 'workers',
        runPoint(): PointRunResult {
          return {
            result: 'failed',
            p95Ms: 500,
            successRate: 0.8,
            droppedIterations: 0,
            completedIterations: 5,
            cleanupRate: 1,
            runnerExitCode: 0,
            summaryPath: 'summary.json',
            reason: 'upload success rate is below minimum',
          }
        },
      }

      const config: BaseCapacityConfig = {
        campaignId: 'test-baseline-fail',
        targetUrl: 'http://localhost',
        maxLevel: 16,
        baselineIterations: 5,
        confirmationIterations: 10,
        latencyMultiplier: 2,
        configuredP95LimitMs: null,
        minimumSuccessRate: 0.95,
      }

      const state = makeCampaignState(mockScenario, config, {
        resultsDirectory: testDir,
      })

      const exitCode = runCapacitySearch(state)
      equal(exitCode, 1)
      equal(state.report.status, 'completed')
      equal(state.report.maximum_capacity, 0)
      equal(state.report.result_bound, 'exact')
    } finally {
      rmSync(testDir, { recursive: true, force: true })
    }
  })

  it('bisects accurately when capacity is reached mid-search', (): void => {
    const testDir = makeTestDir()
    try {
      // Capacity threshold is 5 (levels <= 5 pass, > 5 fail)
      const mockScenario: CapacityScenario<BaseCapacityConfig> = {
        name: 'mock',
        unit: 'workers',
        runPoint(_phase: CapacityPhase, level: number): PointRunResult {
          const passed = level <= 5
          return {
            result: passed ? 'passed' : 'failed',
            p95Ms: passed ? 120 : 350,
            successRate: passed ? 1 : 0.8,
            droppedIterations: 0,
            completedIterations: 10,
            cleanupRate: 1,
            runnerExitCode: 0,
            summaryPath: 'summary.json',
            reason: passed ? '' : 'threshold exceeded',
          }
        },
      }

      const config: BaseCapacityConfig = {
        campaignId: 'test-mid-capacity',
        targetUrl: 'http://localhost',
        maxLevel: 16,
        baselineIterations: 5,
        confirmationIterations: 10,
        latencyMultiplier: 2,
        configuredP95LimitMs: 200,
        minimumSuccessRate: 0.95,
      }

      const state = makeCampaignState(mockScenario, config, {
        resultsDirectory: testDir,
      })

      const exitCode = runCapacitySearch(state)
      equal(exitCode, 0)
      equal(state.report.status, 'completed')
      equal(state.report.maximum_capacity, 5)
      equal(state.report.result_bound, 'exact')
    } finally {
      rmSync(testDir, { recursive: true, force: true })
    }
  })

  it('aborts on infrastructure failure', (): void => {
    const testDir = makeTestDir()
    try {
      const mockScenario: CapacityScenario<BaseCapacityConfig> = {
        name: 'mock',
        unit: 'workers',
        runPoint(): PointRunResult {
          return {
            result: 'infrastructure_failure',
            p95Ms: null,
            successRate: null,
            droppedIterations: null,
            completedIterations: null,
            cleanupRate: null,
            runnerExitCode: 1,
            summaryPath: '',
            reason: 'container failed to start',
          }
        },
      }

      const config: BaseCapacityConfig = {
        campaignId: 'test-infra-fail',
        targetUrl: 'http://localhost',
        maxLevel: 8,
        baselineIterations: 5,
        confirmationIterations: 10,
        latencyMultiplier: 2,
        configuredP95LimitMs: null,
        minimumSuccessRate: 0.95,
      }

      const state = makeCampaignState(mockScenario, config, {
        resultsDirectory: testDir,
      })

      const exitCode = runCapacitySearch(state)
      equal(exitCode, 2)
      equal(state.report.status, 'infrastructure_failure')
    } finally {
      rmSync(testDir, { recursive: true, force: true })
    }
  })
})
