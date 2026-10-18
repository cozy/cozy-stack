export type CapacityPhase =
  | 'baseline'
  | 'discovery'
  | 'discovery-retry'
  | 'discovery-decider'
  | 'confirmation'

export type PointResult = 'passed' | 'failed' | 'infrastructure_failure'
export type ResultBound = 'exact' | 'lower_bound'
export type CampaignStatus = 'running' | 'completed' | 'infrastructure_failure'
export type P95LimitMode = 'absolute' | 'baseline_multiplier'

export type CampaignCriteria = {
  baseline_iterations: number
  maximum_p95_ms: number | null
  minimum_success_rate: number
  maximum_dropped_iterations: number
  p95_baseline_multiplier: number | null
  p95_limit_mode: P95LimitMode
  confirmation_iterations_per_vu: number
  [key: string]: unknown
}

export type CapacityAttempt = {
  phase: CapacityPhase
  level: number
  vus?: number
  iterations_per_vu: number
  p95_ms: number | null
  success_rate: number | null
  dropped_iterations: number | null
  completed_iterations: number | null
  cleanup_rate: number | null
  passed: boolean
  result: PointResult
  reason: string
  runner_exit_code: number
  summary_path: string
}

export type CapacityReport = {
  campaign_id: string
  scenario: string
  target: string
  status: CampaignStatus
  configured_max_level: number
  configured_max_vus?: number
  criteria: CampaignCriteria
  attempts: CapacityAttempt[]
  baseline_p95_ms?: number
  p95_limit_ms?: number
  maximum_capacity?: number
  maximum_concurrent_uploads?: number
  result_bound?: ResultBound
  failure_reason?: string
  [key: string]: unknown
}

export type PointOutcome = {
  p95Ms: number | null
  result: PointResult
}

export type PointRunResult = {
  result: PointResult
  p95Ms: number | null
  successRate: number | null
  droppedIterations: number | null
  completedIterations: number | null
  cleanupRate: number | null
  runnerExitCode: number
  summaryPath: string
  reason: string
}

export interface CapacityScenario<TConfig = unknown> {
  readonly name: string
  readonly unit: string
  validatePreflight?(config: TConfig): void
  runPoint(
    phase: CapacityPhase,
    level: number,
    iterationsPerVu: number,
    p95LimitMs: number | null,
    config: TConfig,
    attemptNumber: number,
    executionId: string,
  ): PointRunResult
  buildReportMetadata?(config: TConfig): Record<string, unknown>
}
