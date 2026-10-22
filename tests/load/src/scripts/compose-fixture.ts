import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { chmodSync, closeSync, lstatSync, mkdirSync, openSync, readFileSync, readSync,
  readdirSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export type Docker = (args: string[], input?: string) => string
const harness = fileURLToPath(new URL('../..', import.meta.url))
const blueprint = resolve(harness, 'compose.fixture.yaml')
const isBlueprintSource = (files: string): boolean => [blueprint, `${blueprint},-`].includes(files)
const usage = 'Usage: compose-fixture.ts up|discard PROJECT; snapshot|restore PROJECT DIRECTORY'

const execute: Docker = (args, input) => {
  const result = spawnSync('docker', args, { cwd: harness, encoding: 'utf8', input,
    maxBuffer: 16 * 1024 * 1024 })
  if (result.error || result.status !== 0) throw new Error('Docker command failed')
  return result.stdout.trim()
}

type Json = Record<string, any>
const services = ['couchdb', 'redis', 'rabbitmq', 'minio', 'cozy-stack']
const mounts: Record<string, Record<string, string>> = {
  couchdb: { 'couchdb-data': '/opt/couchdb/data' }, redis: { 'redis-data': '/data' },
  rabbitmq: { 'rabbitmq-data': '/var/lib/rabbitmq' }, minio: { 'minio-data': '/bitnami/minio/data' },
  'cozy-stack': { 'cozy-stack-config': '/etc/cozy', 'cozy-stack-data': '/var/lib/cozy/data' },
}
const volumes = Object.values(mounts).flatMap(Object.keys)
const projectLabel = 'com.docker.compose.project'
const marker = 'io.cozy.load.fixture'
const imagePattern = /^sha256:[a-f0-9]{64}$/
const desktopBindPattern = /^\/run\/desktop\/mnt\/host\/wsl\/docker-desktop-bind-mounts\/[^/]+\/[a-f0-9]{64}$/
const isDesktopBind = (source: string): boolean => desktopBindPattern.test(source)
const lines = (value: string): string[] => value.trim() ? value.trim().split(/\s+/) : []
class FixtureError extends Error {}
const check = (condition: unknown, message: string): void => { if (!condition) throw new FixtureError(message) }
const same = (a: string[], b: string[]): boolean => JSON.stringify([...a].sort()) === JSON.stringify([...b].sort())

function validateConfig(config: Json, project: string, bind: string): void {
  check(config.name === project && same(Object.keys(config.services ?? {}), services), 'Invalid fixture services')
  check(!config.secrets && !config.configs && !config.include, 'Unsupported fixture configuration')
  check(same(Object.keys(config.volumes ?? {}), volumes), 'Invalid fixture volumes')
  check(same(Object.keys(config.networks ?? {}), ['load-test']), 'Invalid fixture network')
  for (const [key, value] of Object.entries<Json>({ ...config.volumes, ...config.networks })) {
    check(value.labels?.[marker] === 'true', 'Missing fixture resource marker')
    check(value.name === `${project}_${key}` && !value.external && !value.driver_opts &&
      Object.keys(value.ipam ?? {}).length === 0 &&
      (!value.driver || value.driver === (key === 'load-test' ? 'bridge' : 'local')), 'Unscoped fixture resource')
  }
  for (const name of services) {
    const service = config.services[name]
    check(service.labels?.[marker] === 'true', 'Missing fixture service marker')
    check(same(Object.keys(service.networks ?? {}), ['load-test']), 'Unscoped service network')
    for (const key of ['privileged', 'devices', 'volumes_from', 'tmpfs', 'network_mode', 'container_name', 'pid',
      'ipc', 'userns_mode', 'env_file', 'secrets', 'configs', 'external_links', 'links', 'extends']) {
      check(!service[key], 'Unsupported fixture service configuration')
    }
    check(!service.scale || service.scale === 1, 'Unsupported fixture service scale')
    check(!service.deploy?.replicas || service.deploy.replicas === 1, 'Unsupported fixture replicas')
    check(name === 'cozy-stack' || !service.ports?.length, 'Unexpected backend published port')
    const expected = mounts[name]!
    const seen: string[] = []
    for (const mount of service.volumes ?? []) {
      if (mount.type === 'bind') {
        check(name === 'cozy-stack' && mount.source === bind && mount.read_only === true &&
          mount.target === '/cozy-config/cozy.yaml', 'Unexpected fixture bind mount')
        seen.push('bind')
      } else {
        check(mount.type === 'volume' && expected[mount.source] === mount.target && !mount.read_only &&
          !mount.volume?.subpath, 'Unexpected fixture volume mount')
        seen.push(mount.source)
      }
    }
    check(same(seen, [...Object.keys(expected), ...(name === 'cozy-stack' ? ['bind'] : [])]),
      'Missing or duplicate fixture mount')
  }
}

const captured = ['couchdb-data', 'minio-data', 'cozy-stack-config', 'cozy-stack-data']
const bundleFiles = ['config.json', 'stack.yaml', ...captured.map(name => `${name}.tar.gz`)]
const helperScript = `umask 077
volume=/volume
backup=/backup
case "$1" in
  empty) for e in "$volume"/.[!.]* "$volume"/..?* "$volume"/*; do
    if [ -e "$e" ] || [ -L "$e" ]; then exit 1; fi; done ;;
  certs) test -z "$(find "$volume" -mindepth 1 ! -type d -print -quit)" ;;
  capture) tar czpf "$backup/$2" -C "$volume" . && chown "$3" "$backup/$2" ;;
  extract) tar xzpf "$backup/$2" -C "$volume" ;;
  *) exit 1 ;;
esac`

function digest(path: string): string {
  const hash = createHash('sha256')
  const buffer = Buffer.alloc(1024 * 1024)
  const fd = openSync(path, 'r')
  try {
    let size: number
    while ((size = readSync(fd, buffer, 0, buffer.length, null)) > 0) hash.update(buffer.subarray(0, size))
    return hash.digest('hex')
  } finally { closeSync(fd) }
}

export function runFixture(args: string[], docker: Docker = execute): void {
  const [command, project, directory] = args
  if (!['up', 'snapshot', 'restore', 'discard'].includes(command ?? '')) throw new Error(usage)
  if (!project || !/^load-fixture-[a-z0-9]+(?:-[a-z0-9]+)*$/.test(project) || project.length > 63) {
    throw new Error('Invalid fixture project: use load-fixture- followed by a lowercase slug')
  }
  const bundle = command === 'snapshot' || command === 'restore'
  if (args.length !== (bundle ? 3 : 2)) throw new Error(usage)
  if (bundle && (!directory || /[\x00-\x1f,]/.test(directory))) throw new Error('Invalid snapshot directory')
  let phase = 'preflight'
  const call: Docker = (arguments_, input) => {
    try { return docker(arguments_, input).trim() } catch { throw new Error('Docker command failed') }
  }
  const json = (arguments_: string[]): Json => JSON.parse(call(arguments_))
  const compose = (...arguments_: string[]): string[] => [
    'compose', '--project-directory', harness, '-p', project, '-f', blueprint, '--profile', 'cozy', ...arguments_,
  ]
  const inventory = () => {
    const containers = lines(call(['ps', '-aq', '--no-trunc', '--filter', `label=${projectLabel}=${project}`]))
    const named = lines(call(['ps', '-aq', '--no-trunc', '--filter', `name=^/${project}[-_]`]))
    const resource = (kind: string) => [...new Set([
      ...lines(call([kind, 'ls', '--format', '{{.Name}}', '--filter', `label=${projectLabel}=${project}`])),
      ...lines(call([kind, 'ls', '--format', '{{.Name}}'])).filter(name => name.startsWith(`${project}_`)),
    ])]
    return { containers: [...new Set([...containers, ...named])], volumes: resource('volume'),
      networks: resource('network') }
  }
  const fresh = () => {
    const found = inventory()
    check(!found.containers.length && !found.volumes.length && !found.networks.length,
      'Refusing existing target resources; choose a fresh project or explicitly discard the owned target')
  }
  const owned = (partial = false) => {
    const found = inventory()
    const containers: Json[] = found.containers.length ? JSON.parse(call(['inspect', ...found.containers])) : []
    check(containers.length === found.containers.length, 'Fixture container inventory changed')
    const names: string[] = []
    const anonymous: string[] = []
    const expectedVolumes: string[] = []
    for (const container of containers) {
      const labels = container.Config?.Labels ?? {}
      const name = labels['com.docker.compose.service']
      check(labels[projectLabel] === project && labels[marker] === 'true' && services.includes(name),
        'Foreign container or missing fixture marker')
      check(!names.includes(name), 'Duplicate fixture service')
      names.push(name)
      check(same(Object.keys(container.NetworkSettings?.Networks ?? {}), [`${project}_load-test`]),
        'Unexpected container network')
      check(!container.State.Paused && !container.State.Restarting, 'Unstable fixture container')
      const seen: string[] = []
      for (const mount of container.Mounts ?? []) {
        if (mount.Type === 'bind') {
          check(name === 'cozy-stack' && (mount.Source === resolve(harness, 'config/cozy/cozy.yaml') ||
            mount.Source.endsWith('/stack.yaml') || isDesktopBind(mount.Source)) &&
            mount.Destination === '/cozy-config/cozy.yaml' &&
            mount.RW === false, 'Unexpected fixture bind mount')
          seen.push('bind')
          continue
        }
        check(mount.Type === 'volume', 'Unexpected fixture mount type')
        const logical = Object.keys(mounts[name]!).find(key => mounts[name]![key] === mount.Destination)
        const certs = name === 'minio' && mount.Destination === '/certs' && /^[a-f0-9]{64}$/.test(mount.Name)
        check(mount.RW === true && (certs || (logical && mount.Name === `${project}_${logical}`)),
          'Unexpected or foreign fixture volume mount')
        check(same(lines(call(['ps', '-aq', '--no-trunc', '--filter', `volume=${mount.Name}`])), [container.Id]),
          'Fixture volume is shared with another container')
        if (certs) { anonymous.push(mount.Name); seen.push('certs') }
        else { expectedVolumes.push(mount.Name); seen.push(logical!) }
      }
      const expected = [...Object.keys(mounts[name]!), ...(name === 'cozy-stack' ? ['bind'] : [])]
      check(same(seen.filter(key => key !== 'certs'), expected) && seen.filter(k => k === 'certs').length <= 1,
        'Missing or duplicate fixture volume mount')
    }
    check(partial || same(names, services), 'Expected exactly five fixture services')
    for (const name of [...found.volumes, ...anonymous]) {
      const volume = JSON.parse(call(['volume', 'inspect', name]))[0]
      const logical = name.slice(project.length + 1)
      check(volume?.Driver === 'local' && !Object.keys(volume.Options ?? {}).length,
        'Unsupported fixture volume driver')
      check((anonymous.includes(name) && !volume.Labels?.[projectLabel]) || (volumes.includes(logical) &&
        volume.Labels?.[projectLabel] === project && volume.Labels?.['com.docker.compose.volume'] === logical &&
        volume.Labels?.[marker] === 'true'),
      'Unowned fixture volume')
      const users = lines(call(['ps', '-aq', '--no-trunc', '--filter', `volume=${name}`]))
      check(users.every(id => containers.some(c => c.Id === id)), 'Fixture volume is shared')
    }
    check(expectedVolumes.every(name => found.volumes.includes(name)), 'Unlabelled fixture volume')
    for (const name of found.networks) {
      const network = JSON.parse(call(['network', 'inspect', name]))[0]
      check(name === `${project}_load-test` && network?.Labels?.[projectLabel] === project &&
        network.Labels['com.docker.compose.network'] === 'load-test' && network.Labels[marker] === 'true' &&
        network.Driver === 'bridge',
      'Unowned fixture network')
      check(Object.keys(network.Containers ?? {}).every(id => containers.some(c => c.Id === id)),
        'Fixture network is shared')
    }
    check(partial || same(found.volumes, volumes.map(v => `${project}_${v}`)), 'Unexpected fixture volumes')
    check(partial || same(found.networks, [`${project}_load-test`]), 'Missing fixture network')
    return { ...found, containers, anonymous }
  }
  const imageId = (reference: string): string => {
    const id = call(['image', 'inspect', reference, '--format', '{{.Id}}'])
    check(imagePattern.test(id) && (!imagePattern.test(reference) || reference === id),
      'Required immutable fixture image is unavailable')
    return id
  }
  const path = resolve(directory ?? '.')
  const save = (name: string, contents: string | Buffer) => writeFileSync(resolve(path, name), contents,
    { mode: 0o600, flag: 'wx' })
  const helper = (image: string, volume: string, operation: string, archive = '') => {
    const reading = operation !== 'extract'
    call(['run', '--rm', '--network', 'none', '--read-only', '--mount',
      `type=volume,src=${volume},dst=/volume${reading ? ',readonly' : ''}`,
      ...(archive ? ['--mount',
        `type=bind,src=${path},dst=/backup${operation === 'extract' ? ',readonly' : ''}`] : []),
      image, 'sh', '-ec', helperScript, 'fixture-helper', operation, archive,
      `${process.getuid!()}:${process.getgid!()}`])
  }
  const verifyBind = (image: string, container: string, expected: string) => {
    const actual = call(['run', '--rm', '--network', 'none', '--read-only', '--volumes-from',
      `${container}:ro`, image, 'sha256sum', '/cozy-config/cozy.yaml']).split(/\s+/)[0]
    check(actual === digest(expected), 'Mounted Stack configuration differs from the captured config')
  }
  try {
    if (command === 'snapshot') {
      // mkdir without recursive/exist-ok refuses every existing baseline, including symlinks.
      mkdirSync(path, { mode: 0o700 })
      const source = owned()
      for (const container of source.containers) check(
        isBlueprintSource(container.Config.Labels['com.docker.compose.project.config_files']) &&
        container.Mounts.filter((mount: Json) => mount.Type === 'bind').every((mount: Json) =>
          mount.Source === resolve(harness, 'config/cozy/cozy.yaml') || isDesktopBind(mount.Source)),
        'Snapshot source must originate from the dedicated fixture blueprint and its Stack bind')
      const cold = () => {
        const current = owned().containers
        check(current.every(c => c.State.Running === false &&
          source.containers.some(original => original.Id === c.Id && original.Image === c.Image)),
        'Cold fixture containers or images changed, or a service is still running')
        verifyBind(helperImage, stack.Id, resolve(path, 'stack.yaml'))
      }
      const config = json(compose('config', '--format', 'json'))
      validateConfig(config, project, resolve(harness, 'config/cozy/cozy.yaml'))
      phase = 'image availability'
      const helperImage = imageId('node:22-alpine')
      for (const container of source.containers) {
        const service = config.services[container.Config.Labels['com.docker.compose.service']]
        service.image = imageId(container.Image)
        delete service.build
        service.environment = Object.fromEntries((container.Config.Env ?? []).map((entry: string) => {
          const equals = entry.indexOf('=')
          return [entry.slice(0, equals), entry.slice(equals + 1)]
        }))
        const host = container.HostConfig
        for (const [key, field] of Object.entries({ mem_limit: 'Memory', mem_reservation: 'MemoryReservation',
          memswap_limit: 'MemorySwap', cpu_quota: 'CpuQuota', cpu_period: 'CpuPeriod', cpu_shares: 'CpuShares',
          cpuset: 'CpusetCpus', pids_limit: 'PidsLimit' })) {
          if (host[field] !== undefined && host[field] !== null) service[key] = host[field]
        }
        service.cpus = (host.NanoCpus ?? 0) / 1e9
        if (service.deploy?.resources) delete service.deploy.resources.limits
      }
      check(config.services.minio.environment.MINIO_SCHEME !== 'https', 'TLS MinIO snapshots are unsupported')
      save('stack.yaml', readFileSync(resolve(harness, 'config/cozy/cozy.yaml')))
      config.services['cozy-stack'].volumes.find((mount: Json) => mount.type === 'bind').source = 'stack.yaml'
      save('config.json', JSON.stringify(config, null, 2) + '\n')
      const stack = source.containers.find(c => c.Config.Labels['com.docker.compose.service'] === 'cozy-stack')!
      const rabbit = source.containers.find(c => c.Config.Labels['com.docker.compose.service'] === 'rabbitmq')!
      const redis = source.containers.find(c => c.Config.Labels['com.docker.compose.service'] === 'redis')!
      phase = 'publisher stop'
      call(['stop', '--time', '30', stack.Id])
      check(owned().containers.find(c => c.Id === stack.Id)?.State.Running === false,
        'Stack publisher did not stop; no queue-idle claim is possible')
      phase = 'idle queues check'
      const vhosts: Json[] = JSON.parse(call(['exec', rabbit.Id, 'rabbitmqctl', '-q', 'list_vhosts',
        '--formatter', 'json', 'name']))
      check(Array.isArray(vhosts) && vhosts.length > 0, 'Cannot prove RabbitMQ queues idle')
      for (const vhost of vhosts) {
        check(typeof vhost.name === 'string' && /^[/a-zA-Z0-9_.]+$/.test(vhost.name), 'Unsupported RabbitMQ vhost')
        const queues: Json[] = JSON.parse(call(['exec', rabbit.Id, 'rabbitmqctl', '-q', 'list_queues',
          '-p', vhost.name, '--formatter', 'json', 'name', 'messages_ready', 'messages_unacknowledged']))
        check(Array.isArray(queues) && queues.every(q =>
          q.messages_ready === 0 && q.messages_unacknowledged === 0),
          'RabbitMQ queues are not idle; finish jobs before taking a new snapshot')
      }
      check(call(['exec', redis.Id, 'redis-cli', '-n', '0', 'DBSIZE']) === '0',
        'Redis jobs DB0 is not empty; finish jobs before taking a new snapshot')
      phase = 'cold stop'
      call(['stop', '--time', '30', ...source.containers.filter(c => c.Id !== stack.Id).map(c => c.Id)])
      cold()
      phase = 'certificate check'
      for (const volume of source.anonymous) helper(helperImage, volume, 'certs')
      phase = 'archive capture'
      for (const name of captured) {
        helper(helperImage, `${project}_${name}`, 'capture', `${name}.tar.gz`)
        chmodSync(resolve(path, `${name}.tar.gz`), 0o600)
      }
      cold()
      save('manifest.json', JSON.stringify({ version: 1, sourceProject: project, helperImage,
        runtimePolicy: 'empty-redis-db0-and-rabbitmq',
        files: Object.fromEntries(bundleFiles.map(name => [name, {
          sha256: digest(resolve(path, name)), bytes: lstatSync(resolve(path, name)).size,
        }])),
      }, null, 2) + '\n')
    }
    if (command === 'discard') {
      const source = owned(true)
      phase = 'discard containers'
      if (source.containers.length) call(['rm', '--force', '--volumes', ...source.containers.map(c => c.Id)])
      phase = 'discard storage'
      const removing = [...source.volumes, ...source.anonymous]
      if (source.volumes.length) call(['volume', 'rm', ...source.volumes])
      if (source.networks.length) call(['network', 'rm', ...source.networks])
      phase = 'discard verification'
      fresh()
      const remaining = lines(call(['volume', 'ls', '--format', '{{.Name}}']))
      check(removing.every(name => !remaining.includes(name)), 'Discarded fixture volumes still exist')
    }
    if (command === 'restore') {
      const privateFile = (name: string) => {
        const info = lstatSync(resolve(path, name))
        check(info.isFile() && (info.mode & 0o077) === 0 && info.uid === process.getuid!(),
          'Bundle files must be private regular files owned by the current user')
      }
      const info = lstatSync(path)
      check(info.isDirectory() && (info.mode & 0o077) === 0 && info.uid === process.getuid!(),
        'Bundle directory must be private and owned by the current user')
      check(same(readdirSync(path), [...bundleFiles, 'manifest.json']), 'Unexpected bundle files')
      privateFile('manifest.json')
      const manifest: Json = JSON.parse(readFileSync(resolve(path, 'manifest.json'), 'utf8'))
      check(manifest.version === 1 && /^load-fixture-[a-z0-9]+(?:-[a-z0-9]+)*$/.test(manifest.sourceProject) &&
        imagePattern.test(manifest.helperImage) && manifest.runtimePolicy === 'empty-redis-db0-and-rabbitmq' &&
        same(Object.keys(manifest.files ?? {}), bundleFiles), 'Invalid bundle manifest schema')
      for (const name of bundleFiles) {
        privateFile(name)
        const file = manifest.files[name]
        check(/^[a-f0-9]{64}$/.test(file.sha256) && Number.isSafeInteger(file.bytes) && file.bytes >= 0 &&
          lstatSync(resolve(path, name)).size === file.bytes && digest(resolve(path, name)) === file.sha256,
        'Bundle checksum mismatch; do not restore this baseline')
      }
      const config: Json = JSON.parse(readFileSync(resolve(path, 'config.json'), 'utf8'))
      validateConfig(config, manifest.sourceProject, 'stack.yaml')
      for (const service of Object.values<Json>(config.services)) {
        check(imagePattern.test(service.image) && !service.build, 'Bundle must pin immutable service images')
      }
      fresh()
      phase = 'image availability'
      imageId(manifest.helperImage)
      for (const service of Object.values<Json>(config.services)) imageId(service.image)
      config.name = project
      for (const [key, value] of Object.entries<Json>({ ...config.volumes, ...config.networks })) {
        value.name = `${project}_${key}`
      }
      for (const service of Object.values<Json>(config.services)) for (const mount of service.volumes) {
        if (mount.type === 'bind') mount.source = resolve(path, 'stack.yaml')
        else mount.volume = { nocopy: true }
      }
      validateConfig(config, project, resolve(path, 'stack.yaml'))
      // Compose interpolates even JSON; preserve literal dollar signs in resolved secrets/configuration.
      const input = JSON.stringify(config).replace(/\$/g, () => '$$')
      const restoreCompose = (...arguments_: string[]) => call([
        'compose', '--project-directory', path, '-p', project, '-f', '-', '--profile', 'cozy', ...arguments_,
      ], input)
      phase = 'restore creation'
      restoreCompose('create', '--no-build', '--pull', 'never', ...services)
      const target = owned()
      check(target.containers.every(c => c.State.Running === false &&
        c.Image === config.services[c.Config.Labels['com.docker.compose.service']].image),
      'Restore target must be stopped and use the recorded images')
      const restoredStack = target.containers.find(c =>
        c.Config.Labels['com.docker.compose.service'] === 'cozy-stack')!
      verifyBind(manifest.helperImage, restoredStack.Id, resolve(path, 'stack.yaml'))
      phase = 'empty target check'
      for (const name of volumes) helper(manifest.helperImage, `${project}_${name}`, 'empty')
      for (const volume of target.anonymous) helper(manifest.helperImage, volume, 'certs')
      phase = 'archive extraction'
      for (const name of captured) helper(manifest.helperImage, `${project}_${name}`, 'extract', `${name}.tar.gz`)
      check(owned().containers.every(c => c.State.Running === false), 'Restore target unexpectedly started')
      phase = 'restore startup'
      restoreCompose('up', '-d', '--no-build', '--pull', 'never', '--wait', ...services)
      check(owned().containers.every(c => c.State.Running === true), 'Expected five running restored services')
    }
    if (command === 'up') {
      fresh()
      const config = json(compose('config', '--format', 'json'))
      validateConfig(config, project, resolve(harness, 'config/cozy/cozy.yaml'))
      phase = 'image availability'
      const pinned = Object.fromEntries(services.map(name =>
        [name, { image: imageId(config.services[name].image) }]))
      phase = 'startup'
      call(compose('-f', '-', 'up', '-d', '--no-build', '--pull', 'never', '--wait', ...services),
        JSON.stringify({ services: pinned }))
      check(same(lines(call(compose('ps', '--status', 'running', '--services'))), services),
        'Expected five running fixture services after startup')
    }
  } catch (error) {
    const detail = error instanceof FixtureError ? `: ${error.message}` : ''
    throw new Error(`Fixture ${phase} failed${detail}. State retained; inspect the project before retry/discard.`)
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    runFixture(process.argv.slice(2))
    console.log('Fixture command completed')
  } catch (error) {
    console.error(error instanceof Error ? error.message : 'Fixture command failed')
    process.exitCode = 1
  }
}
