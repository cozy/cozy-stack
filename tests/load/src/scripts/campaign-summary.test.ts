import { deepEqual, equal, match } from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  buildGrafanaLink,
  describeStepResult,
  formatCampaignSummary,
  parseEnvironment,
  parseSteps,
  type CampaignStep,
} from './campaign-summary.ts'

const uploadStep: CampaignStep = {
  endedAt: '2026-10-18T21:20:00Z',
  exitCode: 0,
  id: 'run-upload-1m',
  kind: 'upload-capacity',
  name: 'upload-capacity-1M',
  startedAt: '2026-10-18T21:10:00Z',
}

describe('parseSteps', (): void => {
  it('reads one step per tab-separated line and ignores blank lines', (): void => {
    deepEqual(
      parseSteps(
        'upload-capacity-1M\tupload-capacity\trun-upload-1m\t2026-10-18T21:10:00Z\t2026-10-18T21:20:00Z\t0\n\n',
      ),
      [uploadStep],
    )
  })
})

describe('parseEnvironment', (): void => {
  it('reads key=value lines and keeps values containing =', (): void => {
    deepEqual(parseEnvironment('commit=abc123\nlimits=cpus=2\n# comment\n'), {
      commit: 'abc123',
      limits: 'cpus=2',
    })
  })
})

describe('describeStepResult', (): void => {
  it('reports an exact upload capacity', (): void => {
    equal(
      describeStepResult(uploadStep, {
        maximum_concurrent_uploads: 48,
        result_bound: 'exact',
        status: 'completed',
      }),
      'maximum concurrent uploads = 48',
    )
  })

  it('reports a lower bound with the failing reason absent', (): void => {
    equal(
      describeStepResult(uploadStep, {
        maximum_concurrent_uploads: 1024,
        result_bound: 'lower_bound',
        status: 'completed',
      }),
      'maximum concurrent uploads ≥ 1024 (cap reached)',
    )
  })

  it('reports why a capacity campaign stopped', (): void => {
    equal(
      describeStepResult(uploadStep, {
        failure_reason: 'baseline failed because of test infrastructure',
        status: 'infrastructure_failure',
      }),
      'stopped: baseline failed because of test infrastructure',
    )
  })

  it('reports the sustained ramp level and the first failing reasons', (): void => {
    equal(
      describeStepResult(
        { ...uploadStep, kind: 'shared-drive-ramp' },
        {
          levels: [
            { level: 1, reasons: [], status: 'passed' },
            { level: 2, reasons: ['ws auth rate 0.5 is below 0.99'], status: 'failed' },
          ],
          max_sustained_level: 1,
          result_bound: 'exact',
        },
      ),
      'maximum sustained level = 1; level 2 failed: ws auth rate 0.5 is below 0.99',
    )
  })

  it('reports a ramp where every level passed', (): void => {
    equal(
      describeStepResult(
        { ...uploadStep, kind: 'shared-drive-ramp' },
        { levels: [], max_sustained_level: 64, result_bound: 'lower_bound' },
      ),
      'maximum sustained level ≥ 64 (every level passed)',
    )
  })

  it('reports a missing result file through the exit code', (): void => {
    equal(describeStepResult({ ...uploadStep, exitCode: 2 }, null), 'no result file (exit 2)')
    equal(
      describeStepResult({ ...uploadStep, kind: 'prepare', id: '' }, null),
      'done',
    )
  })
})

describe('buildGrafanaLink', (): void => {
  it('opens the matching dashboard on the step campaign and time range', (): void => {
    equal(
      buildGrafanaLink('http://grafana.test/', uploadStep),
      'http://grafana.test/d/twake-drive-k6/twake-drive-k6-overview?var-campaignid=run-upload-1m&from=1792357740000&to=1792358460000',
    )
    match(
      buildGrafanaLink('http://grafana.test', { ...uploadStep, kind: 'shared-drive-ramp' }) ?? '',
      /\/d\/twake-drive-shared-drive\/twake-drive-shared-drive-activity\?var-campaignid=run-upload-1m&/,
    )
  })

  it('has no link for steps without a campaign', (): void => {
    equal(buildGrafanaLink('http://grafana.test', { ...uploadStep, id: '', kind: 'prepare' }), null)
  })
})

describe('formatCampaignSummary', (): void => {
  it('lists the context and one row per step', (): void => {
    const text = formatCampaignSummary({
      environment: { commit: 'abc123', limits: 'cozy-stack cpus=2 mem=4g', topology: 'single-host' },
      grafanaUrl: 'http://grafana.test',
      name: 'quick',
      results: new Map([['run-upload-1m', { maximum_concurrent_uploads: 48, result_bound: 'exact', status: 'completed' }]]),
      runId: '20261018T211000Z',
      steps: [uploadStep, { ...uploadStep, exitCode: 0, id: '', kind: 'prepare', name: 'shared-drive-prepare' }],
    })

    match(text, /^# Campaign quick \(20261018T211000Z\)/)
    match(text, /- Topology: single-host/)
    match(text, /- Commit: abc123/)
    match(text, /- Limits: cozy-stack cpus=2 mem=4g/)
    match(text, /\| upload-capacity-1M \| maximum concurrent uploads = 48 \| \[Grafana\]\(http:\/\/grafana\.test\/d\/twake-drive-k6\//)
    match(text, /\| shared-drive-prepare \| done \| \|/)
  })
})
