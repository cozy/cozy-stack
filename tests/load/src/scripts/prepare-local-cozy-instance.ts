import { spawnSync } from 'node:child_process'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import {
  getEnvironmentFilePath,
  harnessDirectory,
  saveEnvironmentFile,
} from './env.ts'

const stackConfigPath = '/cozy-config/cozy.yaml'

export type CommandResult = {
  status: number
  stderr: string
  stdout: string
}

type JsonRecord = Record<string, unknown>

function isJsonRecord(value: unknown): value is JsonRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function getStackCommandResult(arguments_: string[]): CommandResult {
  const result = spawnSync(
    'docker',
    [
      'compose',
      '--profile',
      'cozy',
      'exec',
      '-T',
      'cozy-stack',
      'cozy-stack',
      '-c',
      stackConfigPath,
      ...arguments_,
    ],
    {
      cwd: harnessDirectory,
      encoding: 'utf8',
      maxBuffer: 10 * 1024 * 1024,
    },
  )

  if (result.error) {
    throw new Error('Could not execute cozy-stack in Docker', {
      cause: result.error,
    })
  }

  return {
    status: result.status ?? 1,
    stderr: result.stderr,
    stdout: result.stdout,
  }
}

function getCommandFailureDetail(result: CommandResult): string {
  return result.stderr.trim() || result.stdout.trim()
}

function throwStackCommandFailure(result: CommandResult): never {
  const detail = getCommandFailureDetail(result)

  throw new Error(
    detail.length > 0
      ? `cozy-stack command failed: ${detail}`
      : 'cozy-stack command failed without an error message',
  )
}

function getSuccessfulStackCommandOutput(arguments_: string[]): string {
  const result = getStackCommandResult(arguments_)

  if (result.status !== 0) {
    throwStackCommandFailure(result)
  }

  return result.stdout.trim()
}

export function isMissingInstanceResult(result: CommandResult): boolean {
  return (
    result.status !== 0 &&
    getCommandFailureDetail(result).endsWith(
      'Error: Not Found: Instance not found',
    )
  )
}

function ensureInstance(instanceDomain: string): 'created' | 'existing' {
  const showResult = getStackCommandResult([
    'instances',
    'show',
    instanceDomain,
  ])

  if (showResult.status === 0) {
    return 'existing'
  }
  if (!isMissingInstanceResult(showResult)) {
    throwStackCommandFailure(showResult)
  }

  getSuccessfulStackCommandOutput([
    'instances',
    'add',
    instanceDomain,
    '--passphrase',
    'cozy',
    '--email',
    'load@localhost',
    '--locale',
    'en',
    '--public-name',
    'Twake Drive load tests',
    '--context-name',
    'dev',
  ])
  return 'created'
}

export function getOAuthClientIdFromJson(output: string): string | null {
  try {
    const parsedOutput: unknown = JSON.parse(output)

    if (!isJsonRecord(parsedOutput)) {
      return null
    }

    const clientId = parsedOutput.client_id ?? parsedOutput._id
    return typeof clientId === 'string' && clientId.length > 0 ? clientId : null
  } catch (_error: unknown) {
    return null
  }
}

export function isMissingOAuthClientResult(
  result: CommandResult,
  softwareId: string,
): boolean {
  return (
    result.status !== 0 &&
    getCommandFailureDetail(result).endsWith(
      `Error: Unqualified error: Could not find client with software_id ${softwareId}`,
    )
  )
}

function ensureOAuthClient(instanceDomain: string, softwareId: string): string {
  const findResult = getStackCommandResult([
    'instances',
    'find-oauth-client',
    instanceDomain,
    softwareId,
  ])

  if (findResult.status === 0) {
    const existingClientId = getOAuthClientIdFromJson(findResult.stdout)

    if (existingClientId !== null) {
      return existingClientId
    }

    throw new Error('cozy-stack returned an invalid OAuth client lookup')
  }

  if (!isMissingOAuthClientResult(findResult, softwareId)) {
    throwStackCommandFailure(findResult)
  }

  const clientId = getSuccessfulStackCommandOutput([
    'instances',
    'client-oauth',
    instanceDomain,
    'http://localhost/twake-load',
    'Twake Drive load tests',
    softwareId,
  ])

  if (clientId.length === 0 || /\s/.test(clientId)) {
    throw new Error('cozy-stack returned an invalid OAuth client ID')
  }

  return clientId
}

function getAccessToken(
  instanceDomain: string,
  clientId: string,
  expiration: string,
): string {
  const accessToken = getSuccessfulStackCommandOutput([
    'instances',
    'token-oauth',
    instanceDomain,
    clientId,
    'io.cozy.files',
    '--expire',
    expiration,
  ])

  if (accessToken.length === 0 || /\s/.test(accessToken)) {
    throw new Error('cozy-stack returned an invalid OAuth access token')
  }

  return accessToken
}

function prepareLocalCozyInstance(): number {
  try {
    const instanceDomain =
      process.env.COZY_INSTANCE_DOMAIN || 'load.localhost:8080'
    const baseUrl = process.env.COZY_BASE_URL || `http://${instanceDomain}`
    const softwareId =
      process.env.COZY_CLIENT_SOFTWARE_ID || 'twake-drive-load-local'
    const tokenExpiration = process.env.COZY_TOKEN_EXPIRATION || '24h'
    const environmentPath = getEnvironmentFilePath()
    const instanceResult = ensureInstance(instanceDomain)
    const clientId = ensureOAuthClient(instanceDomain, softwareId)
    const accessToken = getAccessToken(
      instanceDomain,
      clientId,
      tokenExpiration,
    )

    saveEnvironmentFile(environmentPath, {
      BASE_URL: baseUrl,
      COZY_ACCESS_TOKEN: accessToken,
    })

    process.stdout.write(
      `Cozy instance ${instanceDomain} ${instanceResult}; access token saved to ${environmentPath}\n`,
    )
    return 0
  } catch (error: unknown) {
    const message = error instanceof Error ? error.message : String(error)
    process.stderr.write(`${message}\n`)
    return 1
  }
}

function isMainModule(): boolean {
  const entryPath = process.argv[1]
  return entryPath !== undefined && resolve(entryPath) === fileURLToPath(import.meta.url)
}

if (isMainModule()) {
  process.exitCode = prepareLocalCozyInstance()
}
