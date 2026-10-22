import assert from 'node:assert/strict'
import test from 'node:test'
import { resolve, join } from 'node:path'
import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync,
  rmSync, statSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { tmpdir } from 'node:os'

import { runFixture } from './compose-fixture.ts'
import type { Docker } from './compose-fixture.ts'

const harness = fileURLToPath(new URL('../..', import.meta.url))
const services = ['couchdb', 'redis', 'rabbitmq', 'minio', 'cozy-stack']
const destinations: Record<string, Record<string, string>> = {
  couchdb: { 'couchdb-data': '/opt/couchdb/data' }, redis: { 'redis-data': '/data' },
  rabbitmq: { 'rabbitmq-data': '/var/lib/rabbitmq' }, minio: { 'minio-data': '/bitnami/minio/data' },
  'cozy-stack': { 'cozy-stack-config': '/etc/cozy', 'cozy-stack-data': '/var/lib/cozy/data' },
}
const image = 'sha256:' + 'a'.repeat(64)
const marker = { 'io.cozy.load.fixture': 'true' }
const project = 'load-fixture-test'
const stackBind = resolve(harness, 'config/cozy/cozy.yaml')
const config = () => ({
  name: project,
  services: Object.fromEntries(services.map(service => [service, {
    image, labels: { ...marker }, networks: { 'load-test': null }, environment: {},
    volumes: [
      ...Object.entries(destinations[service]!).map(([source, target]) => ({ type: 'volume', source, target })),
      ...(service === 'cozy-stack'
        ? [{ type: 'bind', source: stackBind, target: '/cozy-config/cozy.yaml', read_only: true }] : []),
    ],
  }])),
  volumes: Object.fromEntries(Object.values(destinations).flatMap(Object.keys)
    .map(name => [name, { name: `${project}_${name}`, labels: { ...marker } }])),
  networks: { 'load-test': { name: `${project}_load-test`, labels: { ...marker } } },
})

test('up pins all five local images through an image-only stdin overlay without pulling or building', () => {
  const calls: { args: string[], input: string | undefined }[] = []
  const actual = config() as Json
  const images = Object.fromEntries(services.map((name, i) => [name, `sha256:${String(i).repeat(64)}`]))
  for (const name of services) actual.services[name].image = `local/${name}:mutable`
  actual.services['cozy-stack'].environment = { CUSTOM: 'secret$literal' }
  actual.services['cozy-stack'].mem_limit = 123456
  const docker: Docker = (args, input) => {
    calls.push({ args, input })
    if (args.includes('config')) return JSON.stringify(actual)
    if (args[0] === 'image') return images[args[2]!.split('/')[1]!.split(':')[0]!]!
    if (args[0] === 'compose' && args.includes('--services')) return services.join('\n')
    return ''
  }
  runFixture(['up', project], docker)
  const up = calls.find(call => call.args.includes('up'))!
  assert.ok(up, 'up must actually be executed')
  assert.deepEqual(up.args.filter((_, i) => up.args[i - 1] === '-f'),
    [resolve(harness, 'compose.fixture.yaml'), '-'])
  assert.ok(up.args.includes('--no-build') && up.args.includes('--wait'))
  assert.equal(up.args[up.args.indexOf('--pull') + 1], 'never')
  assert.ok(up.args.includes('--profile') && up.args.includes('cozy'))
  assert.deepEqual(up.args.slice(-5), services)
  assert.deepEqual(JSON.parse(up.input!), {
    services: Object.fromEntries(services.map(name => [name, { image: images[name] }])),
  })
  const inspected = calls.filter(call => call.args[0] === 'image')
  assert.deepEqual(inspected.map(call => call.args), services.map(name =>
    ['image', 'inspect', `local/${name}:mutable`, '--format', '{{.Id}}']))
  assert.ok(inspected.every(call => calls.indexOf(call) < calls.indexOf(up)))
  assert.equal(actual.services['cozy-stack'].environment.CUSTOM, 'secret$literal')
  assert.equal(actual.services['cozy-stack'].mem_limit, 123456)
  assert.ok(!calls.some(({ args }) => args.includes('build') || args.includes('down') || args.includes('pull')))
})

for (const missing of services) {
  test(`up refuses missing local ${missing} image before any mutation`, () => {
    const actual = config() as Json
    for (const name of services) actual.services[name].image = `local/${name}:mutable`
    const calls: string[][] = []
    assert.throws(() => runFixture(['up', project], args => {
      calls.push(args)
      if (args.includes('config')) return JSON.stringify(actual)
      if (args[0] === 'image') {
        if (args[2] === `local/${missing}:mutable`) throw new Error('password=SUPERSECRET')
        return image
      }
      if (args.includes('--services')) return services.join('\n')
      return ''
    }), error => error instanceof Error && /image availability.*retained/.test(error.message) &&
      !error.message.includes('SUPERSECRET'))
    assert.ok(!calls.some(args => args.some(arg =>
      ['up', 'create', 'build', 'pull', 'rm', 'stop', 'run', 'down'].includes(arg))))
  })
}

test('up accepts the empty IPAM object emitted by real Docker Compose', () => {
  const actual = config() as Json
  actual.networks['load-test'].ipam = {}
  const calls: string[][] = []
  runFixture(['up', project], args => {
    calls.push(args)
    if (args.includes('config')) return JSON.stringify(actual)
    if (args[0] === 'image') return image
    if (args.includes('--services')) return services.join('\n')
    return ''
  })
  assert.ok(calls.some(args => args.includes('up')))
})

test('up rejects nonempty IPAM configuration before creating resources', () => {
  const actual = config() as Json
  actual.networks['load-test'].ipam = { config: [{ subnet: '10.10.0.0/24' }] }
  const calls: string[][] = []
  assert.throws(() => runFixture(['up', project], args => {
    calls.push(args)
    return args.includes('config') ? JSON.stringify(actual) : ''
  }), /resource/)
  assert.ok(!calls.some(args => args.includes('up')))
})

test('up refuses existing resources and never echoes Docker credentials', () => {
  let mutated = false
  assert.throws(() => runFixture(['up', project], args => {
    if (args.includes('config')) return JSON.stringify(config())
    if (args[0] === 'volume' && args[1] === 'ls') return `${project}_couchdb-data`
    if (args.includes('up')) mutated = true
    return ''
  }), /existing|fresh/)
  assert.equal(mutated, false)
  assert.throws(() => runFixture(['up', project], () => {
    throw new Error('password=SUPERSECRET s3://access:private@example')
  }), error => error instanceof Error && !/SUPERSECRET|private|example/.test(error.message))
})

type Json = Record<string, any>

// Only the Docker boundary is simulated; bundles and tar archives are real local files.
function fixture(t: { after: (callback: () => void) => void }) {
  const root = mkdtempSync(join(tmpdir(), 'compose-fixture-test-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const state = {
    containers: [] as Json[], volumes: {} as Record<string, Json>, networks: {} as Record<string, Json>,
    calls: [] as string[][], queues: [] as Json[], redis: '0', certs: false, shared: false,
    bindChecksum: createHash('sha256').update(readFileSync(stackBind)).digest('hex'),
    fail: '', config: config() as Json, input: '',
  }
  const provision = (name: string, running: boolean) => {
    const labels = { 'com.docker.compose.project': name }
    state.networks[`${name}_load-test`] = { Name: `${name}_load-test`, Driver: 'bridge', Labels: {
      ...labels, ...marker, 'com.docker.compose.network': 'load-test' }, Containers: {} }
    for (const service of services) {
      const id = createHash('sha256').update(name + service).digest('hex')
      const named = Object.entries(destinations[service]!).map(([logical, target]) => {
        const volume = `${name}_${logical}`
        const path = join(root, volume)
        mkdirSync(path, { recursive: true })
        state.volumes[volume] = { Name: volume, Driver: 'local', Options: null, Labels: {
          ...labels, ...marker, 'com.docker.compose.volume': logical }, path }
        return { Type: 'volume', Name: volume, Destination: target, RW: true }
      })
      if (service === 'minio') {
        const anonymous = name === project ? 'f'.repeat(64)
          : createHash('sha256').update(name + '/certs').digest('hex')
        const path = join(root, anonymous)
        mkdirSync(path, { recursive: true })
        state.volumes[anonymous] = { Name: anonymous, Driver: 'local', Labels: null, Options: null, path }
        named.push({ Type: 'volume', Name: anonymous, Destination: '/certs', RW: true })
      }
      const container: Json = { Id: id, Name: `/${name}-${service}-1`, Image: image,
        State: { Running: running, Paused: false, Restarting: false },
        Config: { Env: ['CUSTOM=secret$literal'], Labels: { ...labels, ...marker,
          'com.docker.compose.service': service,
          'com.docker.compose.project.config_files': resolve(harness, 'compose.fixture.yaml') } },
        HostConfig: { Memory: 123456, NanoCpus: 2000000000 }, Mounts: named,
        NetworkSettings: { Networks: { [`${name}_load-test`]: {} } } }
      if (service === 'cozy-stack') container.Mounts.push({ Type: 'bind', Source: state.input
        ? JSON.parse(state.input).services['cozy-stack'].volumes.find((v: Json) => v.type === 'bind').source
        : stackBind, Destination: '/cozy-config/cozy.yaml', RW: false })
      state.containers.push(container)
      state.networks[`${name}_load-test`]!.Containers[id] = {}
    }
  }
  provision(project, true)
  const docker: Docker = (args, input) => {
    state.calls.push(args)
    const filter = args[args.indexOf('--filter') + 1] ?? ''
    if (args[0] === 'compose') {
      if (args.includes('config')) return JSON.stringify(state.config)
      if (args.includes('--services')) return services.join('\n')
      if (args.includes('create')) {
        state.input = input ?? ''
        provision(args[args.indexOf('-p') + 1]!, false)
      }
      if (args.includes('up')) for (const container of state.containers) {
        if (container.Config.Labels['com.docker.compose.project'] === args[args.indexOf('-p') + 1]) {
          container.State.Running = true
        }
      }
      return ''
    }
    if (args[0] === 'ps') {
      const selected = state.containers.filter(c => filter.startsWith('volume=')
        ? c.Mounts.some((m: Json) => m.Name === filter.slice(7))
        : filter.startsWith('name=') ? c.Name.startsWith('/' + filter.slice(7).replace('[-_]', ''))
          : c.Config.Labels['com.docker.compose.project'] === filter.split('=').at(-1))
      return [...selected.map(c => c.Id), ...(state.shared && filter.startsWith('volume=')
        ? ['e'.repeat(64)] : [])].join('\n')
    }
    if (args[0] === 'inspect') return JSON.stringify(state.containers.filter(c => args.includes(c.Id)))
    if (args[0] === 'image') return image
    if (args[0] === 'volume' || args[0] === 'network') {
      const records = args[0] === 'volume' ? state.volumes : state.networks
      if (args[1] === 'ls') return Object.keys(records).filter(name => !args.includes('--filter') ||
        records[name]!.Labels?.['com.docker.compose.project'] === filter.split('=').at(-1)).join('\n')
      if (args[1] === 'inspect') return JSON.stringify(args.slice(2).map(name => records[name]))
      if (args[1] === 'rm') for (const name of args.slice(2)) delete records[name]
      return ''
    }
    if (args[0] === 'stop') for (const c of state.containers.filter(c => args.includes(c.Id))) {
      c.State.Running = false
    }
    if (args[0] === 'exec') {
      if (args.includes('list_vhosts')) return '[{"name":"/"}]'
      if (args.includes('list_queues')) return JSON.stringify(state.queues)
      if (args.includes('redis-cli')) return state.redis
    }
    if (args[0] === 'rm') {
      if (args.includes('--volumes')) for (const container of state.containers.filter(c => args.includes(c.Id))) {
        for (const mount of container.Mounts) if (/^[a-f0-9]{64}$/.test(mount.Name ?? '')) {
          delete state.volumes[mount.Name]
        }
      }
      for (const network of Object.values(state.networks)) for (const id of args.slice(1)) {
        delete network.Containers[id]
      }
      state.containers = state.containers.filter(c => !args.includes(c.Id))
    }
    if (args[0] === 'run') {
      if (args.includes('--volumes-from')) return `${state.bindChecksum}  /cozy-config/cozy.yaml`
      const operation = args[args.indexOf('fixture-helper') + 1]
      if (state.fail === operation) throw new Error('password=SUPERSECRET')
      const volume = args.find(a => a.startsWith('type=volume,'))!.split('src=')[1]!.split(',')[0]!
      const path = state.volumes[volume]!.path
      if (operation === 'certs' && state.certs) writeFileSync(join(path, 'private.key'), 'test-only-key')
      const bundle = args.find(a => a.startsWith('type=bind,'))?.split('src=')[1]!.split(',')[0] ?? ''
      const script = args[args.indexOf('-ec') + 1]!.replace('volume=/volume', 'volume="$FIXTURE_VOLUME"')
        .replace('backup=/backup', 'backup="$FIXTURE_BACKUP"')
      const result = spawnSync('sh', ['-ec', script, ...args.slice(args.indexOf('fixture-helper'))], {
        env: { ...process.env, FIXTURE_VOLUME: path, FIXTURE_BACKUP: bundle },
      })
      if (result.status !== 0) throw new Error(result.stderr.toString())
    }
    return ''
  }
  return { state, docker, root, bundle: join(root, 'snapshot') }
}

test('snapshot stops the publisher, refuses dirty queues and retains the stopped source', t => {
  const f = fixture(t)
  f.state.queues = [{ name: 'pending', messages_ready: 1, messages_unacknowledged: 0 }]
  assert.throws(() => runFixture(['snapshot', project, f.bundle], f.docker), /queue|idle/)
  assert.equal(f.state.containers.find(c => c.Config.Labels['com.docker.compose.service'] === 'cozy-stack')!
    .State.Running, false)
  assert.equal(f.state.calls.filter(a => a[0] === 'stop').length, 1)
  assert.ok(!f.state.calls.some(a => a[0] === 'rm' || a.includes('up') || a.includes('capture')))
})

test('snapshot rejects foreign markers and shared volume mounts before stopping anything', t => {
  for (const mutation of ['marker', 'shared', 'mount']) {
    const f = fixture(t)
    if (mutation === 'marker') delete f.state.containers[0]!.Config.Labels['io.cozy.load.fixture']
    if (mutation === 'shared') f.state.shared = true
    if (mutation === 'mount') f.state.containers[0]!.Mounts[0].Name = 'foreign-data'
    assert.throws(() => runFixture(['snapshot', project, f.bundle], f.docker), /marker|shared|volume|mount/)
    assert.ok(!f.state.calls.some(a => a[0] === 'stop' || a[0] === 'run'))
  }
})

test('snapshot writes a private complete bundle only after cold capture with pinned helpers', t => {
  const f = fixture(t)
  writeFileSync(join(f.state.volumes[`${project}_couchdb-data`]!.path, 'global-instances'), 'identity')
  runFixture(['snapshot', project, f.bundle], f.docker)
  assert.deepEqual(readdirSync(f.bundle).sort(), ['config.json', 'stack.yaml', 'manifest.json',
    'couchdb-data.tar.gz', 'minio-data.tar.gz', 'cozy-stack-config.tar.gz', 'cozy-stack-data.tar.gz'].sort())
  assert.equal(statSync(f.bundle).mode & 0o777, 0o700)
  for (const file of readdirSync(f.bundle)) assert.equal(statSync(join(f.bundle, file)).mode & 0o777, 0o600)
  const metadata = JSON.parse(readFileSync(join(f.bundle, 'manifest.json'), 'utf8'))
  assert.equal(metadata.version, 1)
  assert.equal(metadata.helperImage, image)
  const saved = JSON.parse(readFileSync(join(f.bundle, 'config.json'), 'utf8'))
  assert.equal(saved.services['cozy-stack'].environment.CUSTOM, 'secret$literal')
  assert.equal(saved.services['cozy-stack'].mem_limit, 123456)
  assert.equal(saved.services['cozy-stack'].cpus, 2)
  assert.equal(saved.services['cozy-stack'].volumes.find((v: Json) => v.type === 'bind').source, 'stack.yaml')
  assert.ok(f.state.containers.every(c => !c.State.Running))
  for (const call of f.state.calls.filter(a => a[0] === 'run')) {
    assert.ok(call.includes(image) && call.includes('none'))
    if (call.includes('--volumes-from')) {
      assert.ok(call[call.indexOf('--volumes-from') + 1]!.endsWith(':ro'))
      assert.ok(call.includes('--read-only'))
    } else assert.ok(call.find(a => a.startsWith('type=volume,'))!.endsWith(',readonly'))
  }
  f.state.calls = []
  assert.throws(() => runFixture(['snapshot', project, f.bundle], f.docker), /existing|exist|preflight/)
  assert.ok(!f.state.calls.some(a => a[0] === 'stop' || a[0] === 'run'))
})

test('failed capture leaves source stopped and incomplete archives without cleanup or restart', t => {
  const f = fixture(t)
  f.state.fail = 'capture'
  assert.throws(() => runFixture(['snapshot', project, f.bundle], f.docker), error =>
    error instanceof Error && /capture.*retained/i.test(error.message) && !error.message.includes('SUPERSECRET'))
  assert.ok(existsSync(f.bundle))
  assert.ok(!existsSync(join(f.bundle, 'manifest.json')))
  assert.equal(f.state.containers.length, 5)
  assert.ok(f.state.containers.every(c => !c.State.Running))
  assert.ok(!f.state.calls.some(a => a[0] === 'rm' || a.includes('up')))
})

test('snapshot refuses Redis jobs and MinIO certificate files rather than dropping them', t => {
  for (const condition of ['redis', 'certs']) {
    const f = fixture(t)
    if (condition === 'redis') f.state.redis = '1'
    else f.state.certs = true
    assert.throws(() => runFixture(['snapshot', project, f.bundle], f.docker), /idle|cert/i)
    assert.ok(!f.state.calls.some(a => a.includes('capture') || a[0] === 'rm' || a.includes('up')))
  }
})

test('discard removes only inspected owned resources including anonymous certs and reads back absence', t => {
  const f = fixture(t)
  runFixture(['snapshot', project, f.bundle], f.docker)
  const baseline = readFileSync(join(f.bundle, 'manifest.json'))
  runFixture(['discard', project], f.docker)
  assert.equal(f.state.containers.length, 0)
  assert.deepEqual(Object.keys(f.state.volumes), [])
  assert.deepEqual(Object.keys(f.state.networks), [])
  assert.deepEqual(readFileSync(join(f.bundle, 'manifest.json')), baseline)
  assert.ok(!f.state.calls.some(a => a.includes('prune') || a.includes('down')))
})

for (const kind of ['volume', 'network']) for (const value of [undefined, 'false']) {
  test(`discard rejects ${kind} orphan with fixture marker ${value}`, t => {
    const f = fixture(t)
    const name = `${project}_${kind === 'volume' ? 'couchdb-data' : 'load-test'}`
    const resource = (kind === 'volume' ? f.state.volumes : f.state.networks)[name]!
    if (value === undefined) delete resource.Labels['io.cozy.load.fixture']
    else resource.Labels['io.cozy.load.fixture'] = value
    resource.Containers = {}
    f.state.containers = []
    f.state.volumes = kind === 'volume' ? { [name]: resource } : {}
    f.state.networks = kind === 'network' ? { [name]: resource } : {}
    assert.throws(() => runFixture(['discard', project], f.docker), /volume|network|marker/)
    assert.ok(!f.state.calls.some(a => a.includes('rm') || a[0] === 'run' || a.includes('up')))
    assert.equal((kind === 'volume' ? f.state.volumes : f.state.networks)[name], resource)
  })
}

for (const kind of ['volumes', 'networks']) for (const value of [undefined, 'false']) {
  test(`up rejects resolved ${kind} with fixture marker ${value}`, () => {
    const actual = config() as Json
    const resource = Object.values<Json>(actual[kind])[0]!
    if (value === undefined) delete resource.labels['io.cozy.load.fixture']
    else resource.labels['io.cozy.load.fixture'] = value
    const calls: string[][] = []
    assert.throws(() => runFixture(['up', project], args => {
      calls.push(args)
      if (args.includes('config')) return JSON.stringify(actual)
      if (args[0] === 'image') return image
      if (args.includes('--services')) return services.join('\n')
      return ''
    }), /resource|marker/)
    assert.ok(!calls.some(a => a.includes('up') || a.includes('create') || a.includes('rm')))
  })
}

test('blueprint marks all six named volumes and its network as fixture resources', () => {
  const source = readFileSync(resolve(harness, 'compose.fixture.yaml'), 'utf8')
  const resources = ['load-test', ...Object.values(destinations).flatMap(Object.keys)]
  for (const name of resources) {
    assert.match(source, new RegExp(`^  ${name}:\\n    labels: \\*fixture-labels$`, 'm'))
  }
})

test('discard fails closed on unmarked containers, unknown volumes and shared networks', t => {
  for (const condition of ['marker', 'volume', 'network']) {
    const f = fixture(t)
    if (condition === 'marker') f.state.containers[0]!.Config.Labels['io.cozy.load.fixture'] = 'false'
    if (condition === 'volume') f.state.volumes[`${project}_foreign`] = {
      Name: `${project}_foreign`, Driver: 'local', Labels: { 'com.docker.compose.project': project } }
    if (condition === 'network') f.state.networks[`${project}_load-test`]!.Containers.foreign = {}
    assert.throws(() => runFixture(['discard', project], f.docker), /marker|volume|network/)
    assert.ok(!f.state.calls.some(a => a[0] === 'rm' || a.includes('rm')))
  }
})

test('a verified bundle round-trips into fresh storage without rebuilding or mutating its baseline', t => {
  const f = fixture(t)
  const sourceData = f.state.volumes[`${project}_couchdb-data`]!.path
  writeFileSync(join(sourceData, 'global-instances'), 'instance-uuid-preserved')
  runFixture(['snapshot', project, f.bundle], f.docker)
  const baseline = readFileSync(join(f.bundle, 'manifest.json'))
  f.state.calls = []
  const target = 'load-fixture-reset'
  runFixture(['restore', target, f.bundle], f.docker)
  assert.equal(readFileSync(join(f.state.volumes[`${target}_couchdb-data`]!.path, 'global-instances'), 'utf8'),
    'instance-uuid-preserved')
  assert.deepEqual(readFileSync(join(f.bundle, 'manifest.json')), baseline)
  assert.ok(f.state.calls.some(a => a.includes('create')))
  assert.equal(f.state.calls.filter(a => a.includes('extract')).length, 4)
  assert.equal(f.state.calls.filter(a => a.includes('empty')).length, 6)
  assert.ok(f.state.containers.every(c => c.State.Running ===
    (c.Config.Labels['com.docker.compose.project'] === target)))
  const applied = JSON.parse(f.state.input)
  assert.equal(applied.name, target)
  assert.equal(applied.services['cozy-stack'].environment.CUSTOM, 'secret$$literal')
  assert.equal(applied.services['cozy-stack'].volumes.find((v: Json) => v.type === 'bind').source,
    join(f.bundle, 'stack.yaml'))
  assert.ok(Object.values<Json>(applied.services).every(service => service.image === image && !service.build))
  runFixture(['discard', target], f.docker)
  assert.equal(f.state.containers.length, 5)
  assert.equal(readFileSync(join(sourceData, 'global-instances'), 'utf8'), 'instance-uuid-preserved')
  runFixture(['discard', project], f.docker)
  assert.deepEqual(Object.keys(f.state.volumes), [])
})

test('restore validates corruption and existing targets before any resource creation', t => {
  for (const condition of ['corrupt', 'existing', 'config']) {
    const f = fixture(t)
    runFixture(['snapshot', project, f.bundle], f.docker)
    if (condition === 'corrupt') writeFileSync(join(f.bundle, 'couchdb-data.tar.gz'), 'damaged')
    if (condition === 'config') writeFileSync(join(f.bundle, 'config.json'), '{"password":"SUPERSECRET"}')
    f.state.calls = []
    assert.throws(() => runFixture(['restore', condition === 'existing' ? project : 'load-fixture-new', f.bundle],
      f.docker), /checksum|bundle|existing|preflight/)
    assert.ok(!f.state.calls.some(a => a.includes('create') || a[0] === 'run' || a.includes('up')))
  }
})

test('failed extraction retains fresh stopped resources and never retries or rolls back', t => {
  const f = fixture(t)
  runFixture(['snapshot', project, f.bundle], f.docker)
  runFixture(['discard', project], f.docker)
  f.state.fail = 'extract'
  f.state.calls = []
  assert.throws(() => runFixture(['restore', 'load-fixture-failed', f.bundle], f.docker), error =>
    error instanceof Error && /extract.*retained/i.test(error.message) && !error.message.includes('SUPERSECRET'))
  assert.equal(f.state.containers.length, 5)
  assert.ok(f.state.containers.every(c => !c.State.Running))
  assert.equal(f.state.calls.filter(a => a.includes('extract')).length, 1)
  assert.ok(!f.state.calls.some(a => a[0] === 'rm' || a.includes('up')))
})

test('snapshot proves the publisher stopped before inspecting queues', t => {
  const f = fixture(t)
  const stack = f.state.containers.find(c => c.Config.Labels['com.docker.compose.service'] === 'cozy-stack')!
  const docker: Docker = (args, input) => {
    const result = f.docker(args, input)
    if (args[0] === 'stop') stack.State.Running = true
    return result
  }
  assert.throws(() => runFixture(['snapshot', project, f.bundle], docker), /stop/)
  assert.ok(!f.state.calls.some(a => a.includes('list_queues') || a.includes('DBSIZE')))
})

test('discard rejects an anonymous cert volume labelled as another Compose project', t => {
  const f = fixture(t)
  f.state.volumes['f'.repeat(64)]!.Labels = { 'com.docker.compose.project': 'foreign' }
  assert.throws(() => runFixture(['discard', project], f.docker), /volume|ownership/)
  assert.ok(!f.state.calls.some(a => a[0] === 'rm' || a.includes('rm')))
})

test('interrupted discard keeps remaining labelled storage discoverable without orphaning anonymous certs', t => {
  const f = fixture(t)
  const docker: Docker = (args, input) => {
    if (args[0] === 'volume' && args[1] === 'rm') throw new Error('interrupted')
    return f.docker(args, input)
  }
  assert.throws(() => runFixture(['discard', project], docker), /retained/)
  assert.equal(f.state.containers.length, 0)
  assert.equal(f.state.volumes['f'.repeat(64)], undefined)
  assert.equal(Object.keys(f.state.volumes).length, 6)
  runFixture(['discard', project], f.docker)
  assert.deepEqual(Object.keys(f.state.volumes), [])
})

test('snapshot accepts a Docker Desktop bind rewrite only with matching mounted config bytes', t => {
  const f = fixture(t)
  const stack = f.state.containers.find(c => c.Config.Labels['com.docker.compose.service'] === 'cozy-stack')!
  stack.Mounts.find((m: Json) => m.Type === 'bind').Source =
    '/run/desktop/mnt/host/wsl/docker-desktop-bind-mounts/Ubuntu/' + 'a'.repeat(64)
  runFixture(['snapshot', project, f.bundle], f.docker)
  assert.ok(existsSync(join(f.bundle, 'manifest.json')))
})

test('snapshot refuses substituted mounted configuration before archive capture', t => {
  const f = fixture(t)
  const stack = f.state.containers.find(c => c.Config.Labels['com.docker.compose.service'] === 'cozy-stack')!
  stack.Mounts.find((m: Json) => m.Type === 'bind').Source =
    '/run/desktop/mnt/host/wsl/docker-desktop-bind-mounts/Ubuntu/' + 'a'.repeat(64)
  f.state.bindChecksum = 'b'.repeat(64)
  assert.throws(() => runFixture(['snapshot', project, f.bundle], f.docker),
    /Mounted Stack configuration differs/)
  assert.ok(!f.state.calls.some(args => args.includes('capture')))
})

test('snapshot rejects a source image changed during cold stop before writing archives', t => {
  const f = fixture(t)
  const docker: Docker = (args, input) => {
    const result = f.docker(args, input)
    if (args[0] === 'stop' && args.length > 4) f.state.containers[0]!.Image = 'sha256:' + 'b'.repeat(64)
    return result
  }
  assert.throws(() => runFixture(['snapshot', project, f.bundle], docker), /changed|image|cold/)
  assert.ok(!f.state.calls.some(a => a.includes('capture')))
})

test('snapshot accepts dedicated blueprint plus stdin image-overlay provenance', t => {
  const f = fixture(t)
  for (const container of f.state.containers) {
    container.Config.Labels['com.docker.compose.project.config_files'] =
      `${resolve(harness, 'compose.fixture.yaml')},-`
  }
  runFixture(['snapshot', project, f.bundle], f.docker)
  assert.ok(existsSync(join(f.bundle, 'manifest.json')))
})

test('snapshot rejects extra, reordered or substituted source Compose files before stopping', t => {
  const blueprint = resolve(harness, 'compose.fixture.yaml')
  for (const source of [`${blueprint},-,-`, `${blueprint},other.yaml`, `${blueprint},-,other.yaml`,
    `-,${blueprint}`, '-']) {
    const f = fixture(t)
    for (const container of f.state.containers) {
      container.Config.Labels['com.docker.compose.project.config_files'] = source
    }
    assert.throws(() => runFixture(['snapshot', project, f.bundle], f.docker), /blueprint/)
    assert.ok(!f.state.calls.some(args => args[0] === 'stop' || args[0] === 'run'))
  }
})

test('snapshot rejects a blueprint-labelled source whose Stack bind differs from the blueprint', t => {
  const f = fixture(t)
  const stack = f.state.containers.find(c => c.Config.Labels['com.docker.compose.service'] === 'cozy-stack')!
  stack.Mounts.find((m: Json) => m.Type === 'bind').Source = join(f.root, 'stack.yaml')
  assert.throws(() => runFixture(['snapshot', project, f.bundle], f.docker), /bind|blueprint/)
  assert.ok(!f.state.calls.some(a => a[0] === 'stop'))
})

test('restore rejects unsupported resolved mounts before creating any target resources', t => {
  const f = fixture(t)
  runFixture(['snapshot', project, f.bundle], f.docker)
  const configPath = join(f.bundle, 'config.json')
  const saved = JSON.parse(readFileSync(configPath, 'utf8'))
  saved.services['cozy-stack'].tmpfs = ['/unexpected']
  writeFileSync(configPath, JSON.stringify(saved))
  const manifestPath = join(f.bundle, 'manifest.json')
  const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'))
  const contents = readFileSync(configPath)
  manifest.files['config.json'] = { bytes: contents.length,
    sha256: createHash('sha256').update(contents).digest('hex') }
  writeFileSync(manifestPath, JSON.stringify(manifest))
  f.state.calls = []
  assert.throws(() => runFixture(['restore', 'load-fixture-new', f.bundle], f.docker), /configuration|mount/)
  assert.ok(!f.state.calls.some(a => a.includes('create')))
})

test('restore checks bundle privacy and static archive names before contacting Docker', t => {
  for (const condition of ['mode', 'archive']) {
    const f = fixture(t)
    runFixture(['snapshot', project, f.bundle], f.docker)
    if (condition === 'mode') chmodSync(join(f.bundle, 'stack.yaml'), 0o644)
    else {
      const manifestPath = join(f.bundle, 'manifest.json')
      const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'))
      manifest.files['../injection.tar.gz'] = manifest.files['couchdb-data.tar.gz']
      writeFileSync(manifestPath, JSON.stringify(manifest))
    }
    f.state.calls = []
    assert.throws(() => runFixture(['restore', 'load-fixture-new', f.bundle], f.docker), /private|schema/)
    assert.equal(f.state.calls.length, 0)
  }
})

test('restore refuses nonempty runtime storage before extracting any data archive', t => {
  const f = fixture(t)
  runFixture(['snapshot', project, f.bundle], f.docker)
  f.state.calls = []
  const target = 'load-fixture-dirty'
  const docker: Docker = (args, input) => {
    const result = f.docker(args, input)
    if (args.includes('create')) {
      writeFileSync(join(f.state.volumes[`${target}_redis-data`]!.path, '.job'), 'dirty')
    }
    return result
  }
  assert.throws(() => runFixture(['restore', target, f.bundle], docker), /empty target check.*retained/)
  assert.ok(!f.state.calls.some(a => a.includes('extract') || a.includes('up') || a[0] === 'rm'))
  assert.ok(f.state.containers.every(c => !c.State.Running))
})

test('discard does not report success when Docker leaves inspected resources behind', t => {
  const f = fixture(t)
  const docker: Docker = (args, input) => args[0] === 'rm' ? '' : f.docker(args, input)
  assert.throws(() => runFixture(['discard', project], docker), /discard verification.*existing/)
  assert.equal(f.state.containers.length, 5)
})

test('rejects unsafe namespaces and argument counts before calling Docker', () => {
  for (const args of [
    ['up', 'twake-drive-load'], ['discard', 'load-fixture-'], ['up', 'load-fixture-A'],
    ['up', 'load-fixture-a;id'], ['up', 'load-fixture-../a'], ['up', 'load-fixture-a', 'extra'],
    ['snapshot', 'load-fixture-a'], ['restore', 'load-fixture-a', ''], ['unknown', 'load-fixture-a'],
  ]) {
    assert.throws(() => runFixture(args, () => assert.fail('Docker must not run')), /Usage|project|directory/)
  }
})
