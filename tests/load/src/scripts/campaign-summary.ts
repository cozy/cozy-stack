import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { isJsonRecord, type JsonRecord } from './capacity/k6-summary.ts'
import { harnessDirectory } from './env.ts'

// Summarizes one campaign directory written by campaigns/run-campaign.sh:
// one line per step with its result and a Grafana link for its time range.

export type CampaignStepKind = 'upload-capacity' | 'shared-drive-ramp' | 'prepare'

export type CampaignStep = {
  endedAt: string
  exitCode: number
  id: string
  kind: CampaignStepKind | string
  name: string
  startedAt: string
}

export type CampaignSummaryInput = {
  environment: Record<string, string>
  grafanaUrl: string
  name: string
  results: Map<string, JsonRecord>
  runId: string
  steps: CampaignStep[]
}

const GRAFANA_MARGIN_MS = 60_000

const dashboards: Record<string, string> = {
  'shared-drive-ramp': 'twake-drive-shared-drive/twake-drive-shared-drive-activity',
  'upload-capacity': 'twake-drive-k6/twake-drive-k6-overview',
}

export function parseSteps(tsv: string): CampaignStep[] {
  return tsv
    .split('\n')
    .filter((line: string): boolean => line.trim().length > 0)
    .map((line: string): CampaignStep => {
      const [name = '', kind = '', id = '', startedAt = '', endedAt = '', exitCode = ''] =
        line.split('\t')
      return { endedAt, exitCode: Number(exitCode), id, kind, name, startedAt }
    })
}

export function parseEnvironment(text: string): Record<string, string> {
  const environment: Record<string, string> = {}

  for (const line of text.split('\n')) {
    const separator = line.indexOf('=')
    if (line.startsWith('#') || separator <= 0) {
      continue
    }
    environment[line.slice(0, separator)] = line.slice(separator + 1)
  }

  return environment
}

function describeUploadCapacity(result: JsonRecord): string {
  if (result.status !== 'completed') {
    return `stopped: ${String(result.failure_reason ?? result.status)}`
  }

  const maximum = String(result.maximum_concurrent_uploads)
  return result.result_bound === 'lower_bound'
    ? `maximum concurrent uploads ≥ ${maximum} (cap reached)`
    : `maximum concurrent uploads = ${maximum}`
}

function describeRamp(result: JsonRecord): string {
  const maximum = String(result.max_sustained_level)
  if (result.result_bound === 'lower_bound') {
    return `maximum sustained level ≥ ${maximum} (every level passed)`
  }

  const levels = Array.isArray(result.levels) ? result.levels.filter(isJsonRecord) : []
  const firstNotPassed = levels.find((level: JsonRecord): boolean => level.status !== 'passed')
  if (firstNotPassed === undefined) {
    return `maximum sustained level = ${maximum}`
  }

  const reasons = Array.isArray(firstNotPassed.reasons) ? firstNotPassed.reasons.join('; ') : ''
  return `maximum sustained level = ${maximum}; level ${String(firstNotPassed.level)} ${String(firstNotPassed.status)}: ${reasons}`
}

export function describeStepResult(step: CampaignStep, result: JsonRecord | null): string {
  if (step.kind === 'prepare' || step.id === '') {
    return step.exitCode === 0 ? 'done' : `failed (exit ${step.exitCode})`
  }
  if (result === null) {
    return `no result file (exit ${step.exitCode})`
  }

  return step.kind === 'shared-drive-ramp' ? describeRamp(result) : describeUploadCapacity(result)
}

export function buildGrafanaLink(grafanaUrl: string, step: CampaignStep): string | null {
  const dashboard = dashboards[step.kind]
  const from = Date.parse(step.startedAt)
  const to = Date.parse(step.endedAt)

  if (dashboard === undefined || step.id === '' || Number.isNaN(from) || Number.isNaN(to)) {
    return null
  }

  const base = grafanaUrl.replace(/\/+$/, '')
  return `${base}/d/${dashboard}?var-campaignid=${encodeURIComponent(step.id)}&from=${from - GRAFANA_MARGIN_MS}&to=${to + GRAFANA_MARGIN_MS}`
}

export function formatCampaignSummary(input: CampaignSummaryInput): string {
  const { environment } = input
  const rows = input.steps.map((step: CampaignStep): string => {
    const result = describeStepResult(step, input.results.get(step.id) ?? null)
    const link = buildGrafanaLink(input.grafanaUrl, step)
    return `| ${step.name} | ${result} |${link === null ? '' : ` [Grafana](${link})`} |`
  })

  return [
    `# Campaign ${input.name} (${input.runId})`,
    '',
    `- Topology: ${environment.topology ?? 'unknown'}`,
    `- Commit: ${environment.commit ?? 'unknown'}`,
    `- Limits: ${environment.limits ?? 'unknown'}`,
    '',
    '| Step | Result | Dashboard |',
    '| --- | --- | --- |',
    ...rows,
    '',
  ].join('\n')
}

function resultPath(step: CampaignStep): string | null {
  const results = join(harnessDirectory, 'data', 'results')
  if (step.kind === 'upload-capacity') {
    return join(results, `${step.id}-upload-capacity.json`)
  }
  if (step.kind === 'shared-drive-ramp') {
    return join(results, `${step.id}-shared-drive-ramp.json`)
  }
  return null
}

function readResult(step: CampaignStep): JsonRecord | null {
  const path = resultPath(step)
  if (path === null || !existsSync(path)) {
    return null
  }

  try {
    const parsed: unknown = JSON.parse(readFileSync(path, 'utf8'))
    return isJsonRecord(parsed) ? parsed : null
  } catch (_error: unknown) {
    return null
  }
}

function summarizeCampaign(runDirectory: string): number {
  const environment = parseEnvironment(readFileSync(join(runDirectory, 'environment.txt'), 'utf8'))
  const steps = parseSteps(readFileSync(join(runDirectory, 'steps.tsv'), 'utf8'))
  const results = new Map<string, JsonRecord>()

  for (const step of steps) {
    const result = readResult(step)
    if (result !== null) {
      results.set(step.id, result)
    }
  }

  const summary = formatCampaignSummary({
    environment,
    grafanaUrl: process.env.GRAFANA_URL || 'http://localhost:3000',
    name: environment.campaign ?? 'unknown',
    results,
    runId: environment.run_id ?? 'unknown',
    steps,
  })

  writeFileSync(join(runDirectory, 'summary.md'), summary, 'utf8')
  process.stdout.write(summary)
  return 0
}

function isMainModule(): boolean {
  const entryPath = process.argv[1]
  return entryPath !== undefined && resolve(entryPath) === fileURLToPath(import.meta.url)
}

if (isMainModule()) {
  const runDirectory = process.argv[2]
  if (runDirectory === undefined) {
    process.stderr.write('Usage: campaign-summary.ts <campaign directory>\n')
    process.exitCode = 2
  } else {
    process.exitCode = summarizeCampaign(resolve(runDirectory))
  }
}
