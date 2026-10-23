import assert from 'node:assert/strict'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test, { type TestContext } from 'node:test'

import { readResults } from './serve-capacity-results.ts'

const dashboardPath = new URL('../../config/grafana/dashboards/capacity-overview.json', import.meta.url)

type Target = { expr: string, instant: boolean, range: boolean, format: string, refId: string }
type Override = { matcher: { id: string, options: string }, properties: { id: string, value: unknown }[] }
type Panel = {
  id: number
  title: string
  type: string
  description: string
  datasource?: { type: string, uid: string }
  gridPos: { x: number, y: number, w: number, h: number }
  targets?: Target[]
  options: { textMode?: string, mode?: string, content?: string, [key: string]: unknown }
  fieldConfig?: { defaults: Record<string, unknown>, overrides: Override[] }
  transformations?: { id: string, options: Record<string, unknown> }[]
}
type Dashboard = {
  uid: string
  title: string
  panels: Panel[]
  templating: { list: {
    name: string, type: string, multi: boolean, includeAll: boolean, allValue?: string,
    query: { query: string }, datasource: { uid: string },
  }[] }
  links: { url: string, keepTime: boolean, includeVars: boolean }[]
}

function readDashboard(): Dashboard {
  assert.ok(existsSync(dashboardPath), 'the versioned native capacity dashboard must exist')
  return JSON.parse(readFileSync(dashboardPath, 'utf8')) as Dashboard
}

function panelByTitle(dashboard: Dashboard, title: string): Panel {
  const panel = dashboard.panels.find(panel => panel.title === title)
  assert.ok(panel, `missing native panel: ${title}`)
  return panel
}

test('native capacity summaries distinguish upload workers, unmeasured users and recipient batches', () => {
  const dashboard = readDashboard()
  assert.equal(dashboard.uid, 'twake-drive-capacity')
  assert.equal(dashboard.title, 'Stack Capacity Overview')
  assert.equal(new Set(dashboard.panels.map(panel => panel.id)).size, dashboard.panels.length)
  for (const panel of dashboard.panels) {
    assert.ok(['stat', 'table', 'timeseries', 'text'].includes(panel.type), panel.type)
    assert.ok(panel.gridPos.x >= 0 && panel.gridPos.x + panel.gridPos.w <= 24)
    assert.ok(panel.gridPos.h > 0 && panel.gridPos.y >= 0)
    if (panel.targets?.length) assert.deepEqual(panel.datasource, { type: 'prometheus', uid: 'prometheus' })
    if (panel.type === 'text') {
      assert.equal(panel.options.mode, 'markdown')
      assert.doesNotMatch(panel.options.content ?? '', /<iframe|<script|<style|<html/i)
    }
  }
  const upload = panelByTitle(dashboard, 'Upload worker capacity')
  const users = panelByTitle(dashboard, 'Distinct shared-drive users')
  const fanout = panelByTitle(dashboard, 'Group member batch capacity')
  assert.deepEqual([upload.gridPos.y, users.gridPos.y, fanout.gridPos.y], [0, 0, 0])
  assert.equal(users.type, 'text')
  assert.match(users.options.content ?? '', /NOT MEASURED/)
  assert.match(users.options.content ?? '', /one (identity|token)/i)
  assert.equal(users.targets?.length ?? 0, 0, 'one-token activity is not a user capacity metric')
  for (const [panel, kind] of [[upload, 'upload'], [fanout, 'fanout']] as const) {
    assert.equal(panel.type, 'table', 'name-only stats render blank when no report series exists')
    assert.equal(panel.targets?.length, 1)
    assert.equal(panel.targets?.[0]?.expr,
      `cozy_load_result_info{campaign=~"$campaign",run=~"$run",kind="${kind}"}`)
    assert.equal(panel.targets?.[0]?.format, 'table', 'expose each report as a row, not an info gauge')
    assert.equal(panel.fieldConfig?.defaults.noValue, 'No run')
    assert.equal(panel.fieldConfig?.defaults.unit, 'none')
    assert.equal(panel.fieldConfig?.defaults.displayName, undefined)
    assert.equal(panel.options.showHeader, true)
    assert.equal(panel.options.textMode, undefined)
    assert.equal(panel.options.reduceOptions, undefined, 'never collapse different runs into one result')
    assert.deepEqual(panel.transformations?.map(transform => transform.id), ['organize'])
    const organize = panel.transformations![0]!.options
    const excluded = organize.excludeByName as Record<string, boolean>
    for (const field of [
      'Time', 'Value', '__name__', 'job', 'instance', 'campaign', 'kind',
      'scope', 'bound', 'status', 'reason', 'upload_target',
    ]) assert.equal(excluded[field], true, `summary hides ${field}, including the info-series value 1`)
    const visible = kind === 'upload' ? ['result', 'file_size', 'run'] : ['result', 'run']
    for (const field of visible) assert.notEqual(excluded[field], true, `summary retains ${field}`)
    if (kind === 'fanout') assert.equal(excluded.file_size, true)
    const order = organize.indexByName as Record<string, number>
    assert.deepEqual(Object.keys(order).sort((a, b) => order[a]! - order[b]!), visible)
    assert.deepEqual(organize.renameByName, {}, 'result and run labels remain distinct table columns')
  }
  assert.match(upload.description, /not measured in-flight requests/i)
  assert.match(fanout.description, /all N/i)
  assert.match(fanout.description, /access/i)
})

test('one campaign and optional run selection never aggregate or replace absent report evidence', () => {
  const dashboard = readDashboard()
  const variables = dashboard.templating.list
  assert.deepEqual(variables.map(variable => variable.name), ['campaign', 'run'])
  const [campaign, run] = variables
  assert.equal(campaign?.multi, false)
  assert.equal(campaign?.includeAll, false, 'campaigns cannot be silently mixed')
  assert.equal(campaign?.query.query, 'label_values(cozy_load_result_info, campaign)')
  assert.equal(run?.multi, false)
  assert.equal(run?.includeAll, true)
  assert.equal(run?.allValue, '.*')
  assert.equal(run?.query.query, 'label_values(cozy_load_result_info{campaign=~"$campaign"}, run)')
  for (const variable of variables) assert.equal(variable.datasource.uid, 'prometheus')
  const families = [
    'cozy_load_result_info', 'cozy_load_result_value', 'cozy_load_point_info',
    'cozy_load_point_value', 'cozy_load_result_timestamp_seconds',
  ]
  for (const panel of dashboard.panels) {
    for (const target of panel.targets ?? []) {
      assert.ok(families.some(family => target.expr.startsWith(`${family}{`)), target.expr)
      assert.match(target.expr, /campaign=~"\$campaign"/)
      assert.match(target.expr, /run=~"\$run"/)
      assert.equal(target.instant, true)
      assert.equal(target.range, false)
      assert.doesNotMatch(target.expr, /\b(max|min|sum|avg|topk|bottomk|vector|timestamp|time)\s*\(/)
      assert.doesNotMatch(target.expr, /\bor\b|\[[^\]]*\]|\boffset\b/)
      assert.doesNotMatch(target.expr, /status="completed"/, 'inconclusive and running reports must stay visible')
    }
  }
  const semantics = panelByTitle(dashboard, 'Reading the evidence').options.content ?? ''
  for (const state of ['No run', 'NOT MEASURED', 'Inconclusive', 'Running', 'Not recorded']) {
    assert.ok(semantics.includes(state), `explain ${state} without inventing a zero`)
  }
})

test('scenario tables retain scalar labels, criteria and each actual tested point without joins', () => {
  const dashboard = readDashboard()
  const scenarios = [['Upload', 'upload'], ['Group batch', 'fanout'], ['Activity mix', 'shared_drive']] as const
  const suffixes = [
    ['runs - bounds and status', 'cozy_load_result_info', true],
    ['criteria', 'cozy_load_result_value', false],
    ['tested points', 'cozy_load_point_info', true],
    ['point evidence', 'cozy_load_point_value', false],
  ] as const
  for (const [title, kind] of scenarios) {
    for (const [suffix, family, info] of suffixes) {
      const panel = panelByTitle(dashboard, `${title} ${suffix}`)
      assert.equal(panel.type, 'table')
      assert.equal(panel.targets?.length, 1, 'one scalar table frame; no fragile label joins')
      const target = panel.targets![0]!
      assert.equal(target.expr, `${family}{campaign=~"$campaign",run=~"$run",kind="${kind}"}`)
      assert.equal(target.format, 'table', 'instant Prometheus table responses already expose labels as fields')
      assert.equal(panel.fieldConfig?.defaults.noValue, 'Not recorded')
      assert.deepEqual(panel.transformations?.map(transform => transform.id), ['organize'])
      const organize = panel.transformations![0]!.options
      const excluded = organize.excludeByName as Record<string, boolean>
      assert.equal(excluded.Time, true, 'query evaluation time is not report or point time')
      assert.equal(excluded.Value === true, info, 'hide only the info gauge; retain measured evidence')
      for (const field of ['campaign', 'run', 'kind']) assert.notEqual(excluded[field], true)
      const fields = info && family.includes('result')
        ? ['status', 'bound', 'result', 'reason', 'scope', 'file_size', 'upload_target']
        : family.includes('point') ? ['index', 'load', 'phase', info ? 'status' : 'metric'] : ['metric']
      for (const field of fields) assert.notEqual(excluded[field], true, `${title}: keep ${field}`)
    }
  }
  const criteria = panelByTitle(dashboard, 'Group batch criteria').description
  assert.match(criteria, /budget_ms/)
  assert.match(criteria, /stable_ms/)
  const evidence = panelByTitle(dashboard, 'Group batch point evidence').description
  assert.match(evidence, /owner_ready/)
  assert.match(evidence, /recipient_access_proofs/)
  assert.match(panelByTitle(dashboard, 'Group batch revoke evidence').description, /not started.*not success/i)
  assert.match(panelByTitle(dashboard, 'Activity mix runs - bounds and status').description, /one identity/)
  assert.ok(panelByTitle(dashboard, 'Activity mix runs - bounds and status').gridPos.y > 0)
})

test('result tables put conclusions before repeated run metadata', () => {
  const dashboard = readDashboard()
  for (const scenario of ['Upload', 'Group batch', 'Activity mix']) {
    const panel = panelByTitle(dashboard, `${scenario} runs - bounds and status`)
    const order = panel.transformations![0]!.options.indexByName as Record<string, number>
    assert.deepEqual(Object.keys(order).sort((a, b) => order[a]! - order[b]!), [
      'result', 'status', 'bound', 'reason', 'file_size', 'upload_target', 'scope', 'campaign', 'run', 'kind',
    ], `${scenario}: classified conclusion must not be offscreen behind metadata`)
  }
})

test('scalar criteria and evidence put metrics and measured values before repeated metadata', () => {
  const panels = readDashboard().panels.filter(panel =>
    panel.targets?.some(target => /^cozy_load_(result|point)_value\{/.test(target.expr)))
  assert.deepEqual(panels.map(panel => panel.title).sort(), [
    'Activity mix criteria', 'Activity mix point evidence', 'Group batch criteria',
    'Group batch point evidence', 'Group batch revoke evidence', 'Upload criteria', 'Upload point evidence',
  ])
  for (const panel of panels) {
    assert.deepEqual(panel.transformations?.map(transform => transform.id), ['organize'])
    const organize = panel.transformations![0]!.options
    const order = organize.indexByName as Record<string, number>
    const pointFields = panel.targets![0]!.expr.startsWith('cozy_load_point_value{')
      ? ['load', 'index', 'phase'] : []
    assert.deepEqual(Object.keys(order).sort((a, b) => order[a]! - order[b]!), [
      'metric', 'Value', ...pointFields, 'campaign', 'run', 'kind',
    ], `${panel.title}: numeric evidence must not be offscreen behind metadata`)
    assert.deepEqual(organize.excludeByName, { Time: true, __name__: true, job: true, instance: true },
      'reordering must not hide any evidence or run metadata')
    assert.deepEqual(organize.renameByName, {})
  }
})

test('freshness uses report mtime and missing provenance stays explicit', () => {
  const dashboard = readDashboard()
  const freshness = panelByTitle(dashboard, 'Report update time (filesystem mtime)')
  assert.equal(freshness.type, 'table')
  assert.equal(freshness.targets?.[0]?.expr,
    'cozy_load_result_timestamp_seconds{campaign=~"$campaign",run=~"$run"} * 1000')
  assert.equal(freshness.fieldConfig?.defaults.unit, 'none', 'campaign, run and kind are text, not dates')
  assert.deepEqual(freshness.fieldConfig?.overrides, [{
    matcher: { id: 'byName', options: 'Report updated at' },
    properties: [{ id: 'unit', value: 'dateTimeAsIso' }],
  }], 'only the renamed numeric report timestamp receives date formatting')
  assert.deepEqual(freshness.transformations?.map(transform => transform.id), ['organize'])
  const organize = freshness.transformations![0]!.options
  assert.deepEqual(organize.renameByName, { Value: 'Report updated at' })
  const excluded = organize.excludeByName as Record<string, boolean>
  for (const field of ['campaign', 'run', 'kind', 'Value']) assert.notEqual(excluded[field], true)
  assert.match(freshness.description, /not test start|not run start/i)
  const provenance = panelByTitle(dashboard, 'Recorded environment and timing')
  assert.match(provenance.options.content ?? '', /Not recorded/)
  assert.match(provenance.options.content ?? '', /version.*resources.*revision/i)
  assert.match(provenance.description, /start.*end.*Not recorded/i)
  assert.doesNotMatch(JSON.stringify(dashboard), /localhost|127\.0\.0\.1|file:\/\/|demo|synthetic/i)

  const scope = panelByTitle(dashboard, 'Diagnostic scope').options.content ?? ''
  assert.match(scope, /campaign only/i, 'the old activity dashboard has no execution variable')
  assert.match(scope, /selected.*range/i)
})

test('the provisioned layout has no overlapping panels and both owned sources fit 115 columns', () => {
  const dashboard = readDashboard()
  for (const [index, panel] of dashboard.panels.entries()) {
    for (const other of dashboard.panels.slice(index + 1)) {
      const a = panel.gridPos
      const b = other.gridPos
      const overlaps = a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h
      assert.equal(overlaps, false, `${panel.title} overlaps ${other.title}`)
    }
  }
  for (const source of [dashboardPath, new URL(import.meta.url)]) {
    const wide = readFileSync(source, 'utf8').split('\n').flatMap((line, index) =>
      line.length > 115 ? [`${index + 1}: ${line.length} columns`] : [])
    assert.deepEqual(wide, [], `${source.pathname} must remain reviewable`)
  }
})

test('dashboard fixtures do not depend on a machine-specific temporary directory', () => {
  assert.doesNotMatch(readFileSync(new URL(import.meta.url), 'utf8'), /['"]\/(home|Users)\/[^/]+\//)
})

function linkResults(t: TestContext) {
  const root = mkdtempSync(join(tmpdir(), 'capacity-dashboard-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  for (const run of ['standalone-upload', 'parent-upload']) {
    writeFileSync(join(root, `${run}-upload-capacity.json`),
      JSON.stringify({ campaign_id: run }))
  }
  writeFileSync(join(root, 'parent-activity-shared-drive-ramp.json'), '{}')
  const campaign = join(root, 'campaigns', 'parent')
  mkdirSync(campaign, { recursive: true })
  writeFileSync(join(campaign, 'environment.txt'), 'run_id=parent\n')
  writeFileSync(join(campaign, 'steps.tsv'), [
    'upload\tupload-capacity\tparent-upload\t\t\t0',
    'activity\tshared-drive-ramp\tparent-activity\t\t\t0',
  ].join('\n'))
  const snapshot = readResults(root)
  assert.deepEqual(snapshot.counts, { valid: 3, invalid: 0, unsupported: 0 })
  return snapshot.reports
}

function rowTelemetryUrl(row: { campaign: string, run: string, kind: string }, selectedRun = row.run): URL {
  const scenario = row.kind === 'upload' ? 'Upload' : 'Activity mix'
  const panel = panelByTitle(readDashboard(), `${scenario} runs - bounds and status`)
  const run = panel.fieldConfig?.overrides.find(override =>
    override.matcher.id === 'byName' && override.matcher.options === 'run')
  const links = run?.properties.find(property => property.id === 'links')?.value as
    { url: string, targetBlank: boolean }[] | undefined
  assert.equal(links?.length, 1, 'drill down from an actual report row, not a campaign-wide guess')
  assert.equal(links![0]!.targetBlank, false)
  const expanded = links![0]!.url.replaceAll('${__value.raw}', row.run)
    .replaceAll('$campaign', row.campaign).replaceAll('$run', selectedRun)
    .replace('${__url_time_range}', 'from=1000&to=2000')
  const url = new URL(expanded, 'https://grafana.invalid')
  assert.equal(url.searchParams.get('from'), '1000',
    'the link supplies ?, because the rendered Grafana time-range token does not')
  assert.equal(url.searchParams.get('to'), '2000', 'preserve selected time, never infer it from report mtime')
  return url
}

function uploadTelemetry(run: string) {
  const makefile = readFileSync(new URL('../../Makefile', import.meta.url), 'utf8')
  const recipe = makefile.split('upload: upload-binary observability-up\n')[1]!.split('\nupload-soak:')[0]!
  assert.ok(recipe.includes('--tag campaignid="$(or $(CAPACITY_RUN_ID),$(EXECUTION_ID))"'))
  assert.ok(recipe.includes('--tag executionid="$(EXECUTION_ID)"'))
  const producer = readFileSync(new URL('./find-upload-capacity.ts', import.meta.url), 'utf8')
  assert.ok(producer.includes('`CAPACITY_RUN_ID=${config.campaignId}`'))
  assert.ok(producer.includes('`EXECUTION_ID=${executionId}`'))
  const template = producer.match(/const executionId = `([^`]+)`/)?.[1]
  assert.ok(template, 'derive attempt IDs from the actual upload producer')
  return ['baseline', 'confirmation'].map((phase, index) => ({
    campaignid: run,
    executionid: template.replace('${config.campaignId}', run).replace('${phase}', phase)
      .replace('${vus}', '8').replace('${state.attemptNumber}', String(index + 1)),
  }))
}

function assertUploadSelection(url: URL, run: string) {
  assert.equal(url.pathname, '/d/twake-drive-k6')
  const target = JSON.parse(readFileSync(
    new URL('../../config/grafana/dashboards/k6-overview.json', import.meta.url), 'utf8')) as Dashboard
  const execution = target.templating.list.find(variable => variable.name === 'executionid')
  assert.equal(execution?.includeAll, true)
  assert.equal(execution?.allValue, undefined, 'use Grafana All, not an escaped literal regex value')
  for (const tags of uploadTelemetry(run)) {
    assert.notEqual(tags.executionid, run, 'a report run is not an individual k6 execution')
    assert.equal(url.searchParams.get('var-campaignid'), tags.campaignid)
  }
  assert.equal(url.searchParams.get('var-executionid'), '$__all', 'include every phase/VU/attempt')
}

test('standalone upload row links select its producer campaign and all attempt executions', t => {
  const row = linkResults(t).find(result => result.run === 'standalone-upload')!
  assert.equal(row.campaign, row.run)
  assertUploadSelection(rowTelemetryUrl(row), row.run)
})

test('grouped upload row links select the step campaign, not the reader parent campaign', t => {
  const row = linkResults(t).find(result => result.run === 'parent-upload')!
  assert.equal(row.campaign, 'parent', 'real steps.tsv membership changes only the reader campaign')
  assertUploadSelection(rowTelemetryUrl(row), row.run)
})

test('grouped activity row links select the producer step without an unsupported execution filter', t => {
  const row = linkResults(t).find(result => result.run === 'parent-activity')!
  assert.equal(row.campaign, 'parent')
  const makefile = readFileSync(new URL('../../Makefile', import.meta.url), 'utf8')
  const recipe = makefile.split('cozy-shared-drive-ramp: upload-binary observability-up\n')[1]!
    .split('\ncozy-capacity:')[0]!
  assert.ok(recipe.includes('--tag campaignid="$(EXECUTION_ID)"'))
  assert.ok(!recipe.includes('--tag executionid='))
  const target = JSON.parse(readFileSync(
    new URL('../../config/grafana/dashboards/shared-drive.json', import.meta.url), 'utf8')) as Dashboard
  assert.ok(!target.templating.list.some(variable => variable.name === 'executionid'))
  const url = rowTelemetryUrl(row)
  assert.equal(url.pathname, '/d/twake-drive-shared-drive')
  assert.equal(url.searchParams.get('var-campaignid'), row.run, 'EXECUTION_ID is the step/report run')
  assert.equal(url.searchParams.has('var-executionid'), false)
})

test('All-run selection offers only row-scoped links and cannot mix unrelated producer campaigns', t => {
  const dashboard = readDashboard()
  assert.deepEqual(dashboard.links, [], 'a parent campaign cannot map to one k6 campaignid')
  assert.equal(dashboard.templating.list.find(variable => variable.name === 'run')?.allValue, '.*')
  const rows = linkResults(t).filter(result => result.campaign === 'parent')
  assert.deepEqual(rows.map(row => row.run).sort(), ['parent-activity', 'parent-upload'])
  for (const row of rows) {
    const url = rowTelemetryUrl(row, '.*')
    assert.equal(url.searchParams.get('var-campaignid'), row.run)
    assert.notEqual(url.searchParams.get('var-campaignid'), 'standalone-upload')
    if (row.kind === 'upload') assertUploadSelection(url, row.run)
    else assert.equal(url.searchParams.has('var-executionid'), false)
  }
})
