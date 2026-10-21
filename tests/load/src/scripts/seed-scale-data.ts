import { createHash, randomBytes, randomUUID } from 'node:crypto'
import * as http from 'node:http'
import * as https from 'node:https'
import { isIP } from 'node:net'

import { createSharedDrive } from './prepare-shared-drive.ts'

class CliError extends Error {}
function check(value: unknown, message: string): asserts value {
  if (!value) throw new CliError(message)
}

function integer(name: string, fallback: string, max: number): number {
  const raw = process.env[name] ?? fallback
  const value = Number(raw)
  check(/^\d+$/.test(raw) && Number.isSafeInteger(value) && value <= max,
    `${name} must be an integer between 0 and ${max}`)
  return value
}

function validateDomain(domain: string): void {
  const [host = '', port, ...extra] = domain.split(':')
  const label = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/
  const validPort = port === undefined || (/^[1-9]\d{0,4}$/.test(port) && Number(port) <= 65535)
  check(/^seed-[a-f0-9]{32}\./.test(host) && host.length <= 253 &&
    host.split('.').every((part) => label.test(part)) && !extra.length && validPort,
  'Domain must be seed-<32 lowercase hex>.<DNS suffix>[:port]')
}

function origin(name: string): URL {
  const raw = process.env[name] ?? ''
  try {
    const url = new URL(raw)
    check(/^https?:\/\/[^/?#@\\\s]+\/?$/i.test(raw) &&
      ['http:', 'https:'].includes(url.protocol) && !url.username && !url.password &&
      url.pathname === '/' && !url.search && !url.hash, '')
    return url
  } catch {
    throw new CliError(`${name} must be an HTTP(S) origin without credentials, path, query or fragment`)
  }
}

type Reply = { status: number, body: string }
function request(
  url: URL, method: string, headers: Record<string, string>, timeout: number, body: string | Buffer = '',
): Promise<Reply> {
  let deadline: NodeJS.Timeout | undefined
  return new Promise<Reply>((resolve, reject) => {
    let response: http.IncomingMessage | undefined
    const transport = url.protocol === 'https:' ? https : http
    const hostname = url.hostname.replace(/^\[|\]$/g, '')
    const req = transport.request(url, {
      method, headers: { ...headers, 'Content-Length': String(Buffer.byteLength(body)) },
      // Otherwise HTTPS derives TLS identity from the overridden tenant Host header.
      ...(url.protocol === 'https:' ? { servername: isIP(hostname) ? '' : hostname } : {}),
    }, (res) => {
      response = res
      const chunks: Buffer[] = []
      let size = 0
      res.on('data', (chunk: Buffer) => {
        size += chunk.length
        if (size > 65_536) {
          reject(new CliError('API response exceeds 64 KiB'))
          res.destroy()
          req.destroy()
        } else chunks.push(chunk)
      })
      res.on('end', () => resolve({ status: res.statusCode ?? 0, body: Buffer.concat(chunks).toString('utf8') }))
      res.on('aborted', () => reject(new CliError('API response incomplete')))
      res.on('error', () => reject(new CliError('API response failed')))
    })
    req.on('error', () => reject(new CliError('API request failed')))
    deadline = setTimeout(() => {
      reject(new CliError('API request deadline exceeded'))
      response?.destroy()
      req.destroy()
    }, timeout)
    req.end(body)
  }).finally(() => clearTimeout(deadline))
}

function expectStatus(reply: Reply, status: number): Reply {
  check(reply.status === status, `Unexpected API status: HTTP ${reply.status}`)
  return reply
}

interface Resource {
  id: string
  type?: string
  attributes?: { domain?: string, uuid?: string, type?: string }
  relationships?: { referenced_by?: { data?: Array<{ type?: string, id?: string }> } }
}
function resource(reply: Reply): Resource {
  let data: Resource | undefined
  try { data = JSON.parse(reply.body)?.data } catch { throw new CliError('Invalid API JSON') }
  check(data && typeof data.id === 'string' && data.id.trim(), 'Invalid API resource')
  return data
}

async function main(): Promise<void> {
  const [command, suppliedDomain, ...extra] = process.argv.slice(2)
  check(['seed', 'cleanup'].includes(command ?? '') && !extra.length &&
    (command !== 'cleanup' || suppliedDomain),
  'Usage: seed [domain] | cleanup <domain>; credentials belong in environment variables')
  check(suppliedDomain || process.env.SEED_DOMAIN_SUFFIX,
    'SEED_DOMAIN_SUFFIX is required when no seed domain is supplied')
  const domain = suppliedDomain ?? `seed-${randomBytes(16).toString('hex')}.${process.env.SEED_DOMAIN_SUFFIX}`
  validateDomain(domain)
  const adminURL = origin('SEED_ADMIN_URL')
  const username = process.env.SEED_ADMIN_USERNAME ?? 'admin'
  const password = process.env.SEED_ADMIN_PASSWORD
  check(password && username && !/[:\x00-\x1f\x7f]/.test(username),
    'SEED_ADMIN_PASSWORD and a valid SEED_ADMIN_USERNAME are required')
  const timeout = integer('SEED_REQUEST_TIMEOUT_MS', '1800000', 2_147_483_647)
  check(timeout, 'SEED_REQUEST_TIMEOUT_MS must be positive')
  const authorization = `Basic ${Buffer.from(`${username}:${password}`).toString('base64')}`
  const admin = (method: string, path: string, params?: Record<string, string>) => {
    const url = new URL(path, adminURL)
    if (params) url.search = new URLSearchParams(params).toString()
    return request(url, method, { Authorization: authorization }, timeout)
  }
  // Echo's domain route does not decode %3A; preserve the validated literal public port.
  const instancePath = `/instances/${domain}`
  if (command === 'cleanup') {
    const deleted = await admin('DELETE', instancePath)
    check(deleted.status === 204 || deleted.status === 404, `Cleanup failed: HTTP ${deleted.status}`)
    const readback = await admin('GET', instancePath)
    check(readback.status === 404, `Cleanup absence not confirmed: HTTP ${readback.status}`)
    console.log(`Cleanup complete: ${domain}`)
    return
  }
  const stackURL = origin('SEED_STACK_URL')
  const files = integer('SEED_FILES', '100', 1_000_000)
  const drives = integer('SEED_DRIVES', '0', 100_000)
  check(files || drives, 'At least one file or drive is required')
  console.log(`Seed instance: ${domain}`)
  try {
    const absent = await admin('GET', instancePath)
    check(absent.status === 404, absent.status === 200 ? 'Instance already exists' :
      `Instance lookup failed: HTTP ${absent.status}`)
    const uuid = randomUUID()
    const created = resource(expectStatus(await admin('POST', '/instances', {
      Domain: domain, UUID: uuid, Locale: 'en', Email: `${uuid}@seed.invalid`,
      PublicName: 'Disposable seed fixture', CouchCluster: '-1', SwiftLayout: '-1', AutoUpdate: 'false',
    }), 201))
    const matches = (item: Resource) => item.attributes?.domain === domain && item.attributes?.uuid === uuid
    check(matches(created), 'Created instance identity mismatch')
    const persisted = resource(expectStatus(await admin('GET', instancePath), 200))
    check(matches(persisted) && persisted.id === created.id, 'Instance readback identity mismatch')
    const accessToken = expectStatus(await admin('POST', '/instances/token', {
      Domain: domain, Audience: 'cli', Scope: 'io.cozy.files io.cozy.sharings',
    }), 200).body
    check(accessToken && !/[\s\x00-\x1f\x7f]/.test(accessToken), 'Invalid CLI token')
    const publicAPI = (method: string, path: string, body: string | Buffer = '', headers = {}) =>
      request(new URL(path, stackURL), method, {
        ...headers, Host: domain, Authorization: `Bearer ${accessToken}`,
      }, timeout, body)
    const body = Buffer.alloc(1024, 's')
    const md5 = createHash('md5').update(body).digest('base64')
    for (let index = 1; index <= files; index++) {
      const query = new URLSearchParams({ Type: 'file', Name: `seed-file-${index}.txt` })
      resource(expectStatus(await publicAPI('POST', `/files/io.cozy.files.root-dir?${query}`, body, {
        'Content-Type': 'text/plain', 'Content-MD5': md5,
      }), 201))
    }
    for (let index = 1; index <= drives; index++) {
      const drive = await createSharedDrive({
        baseUrl: stackURL.origin, accessToken, driveName: `Seed shared drive ${index}`,
        fetcher: async (input, init) => {
          const headers = Object.fromEntries(new Headers(init?.headers))
          const path = new URL(String(input)).pathname
          const reply = expectStatus(await publicAPI(init?.method ?? 'GET', path,
            String(init?.body ?? ''), headers), 201)
          return new Response(reply.body, { status: reply.status })
        },
      })
      check(typeof drive.sharingId === 'string' && drive.sharingId.trim() &&
        typeof drive.rootDirectoryId === 'string' && drive.rootDirectoryId.trim(), 'Invalid shared-drive identity')
      const root = resource(expectStatus(await publicAPI('GET',
        `/files/${encodeURIComponent(drive.rootDirectoryId)}`), 200))
      const refs = root.relationships?.referenced_by?.data
      const backlink = Array.isArray(refs) && refs.some((ref) =>
        ref?.type === 'io.cozy.sharings' && ref.id === drive.sharingId)
      check(root.id === drive.rootDirectoryId && root.type === 'io.cozy.files' &&
        root.attributes?.type === 'directory' && backlink, 'Shared-drive root or backlink mismatch')
    }
    console.log(`Seed complete: ${domain}; files=${files}; drives=${drives}`)
  } catch (error) {
    const message = error instanceof CliError ? error.message : 'Seed API operation failed'
    throw new CliError(`${message}. Instance ${domain} may be partial; ` +
      'run explicit cleanup only after server writes have stopped (a timeout/disconnect is not proof).')
  }
}

main().catch((error: unknown) => {
  console.error(error instanceof CliError ? error.message : 'Seed command failed')
  process.exitCode = 1
})
