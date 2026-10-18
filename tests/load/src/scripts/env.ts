import {
  chmodSync,
  existsSync,
  readFileSync,
  renameSync,
  unlinkSync,
  writeFileSync,
} from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDirectory = dirname(fileURLToPath(import.meta.url))
export const harnessDirectory = resolve(scriptDirectory, '..', '..')

export type EnvironmentUpdates = Record<string, string>

export function getEnvironmentFilePath(): string {
  return resolve(
    harnessDirectory,
    process.env.COZY_ENV_FILE || '.env',
  )
}

export function getUpdatedEnvironmentLines(
  existingLines: string[],
  updates: EnvironmentUpdates,
): string[] {
  const remainingKeys = new Set(Object.keys(updates))
  const updatedLines = existingLines.map((line: string): string => {
    const trimmed = line.trim()

    if (trimmed.length === 0 || trimmed.startsWith('#')) {
      return line
    }

    const separatorIndex = line.indexOf('=')
    if (separatorIndex === -1) {
      return line
    }

    const key = line.slice(0, separatorIndex).trim()
    if (Object.prototype.hasOwnProperty.call(updates, key)) {
      remainingKeys.delete(key)
      return `${key}=${updates[key]}`
    }

    return line
  })

  for (const remainingKey of remainingKeys) {
    updatedLines.push(`${remainingKey}=${updates[remainingKey]}`)
  }

  return updatedLines
}

export function saveEnvironmentFile(
  environmentPath: string,
  updates: EnvironmentUpdates,
): string {
  for (const value of Object.values(updates)) {
    if (/\r|\n/.test(value)) {
      throw new Error('Environment values must stay on one line')
    }
  }

  const existingContent = existsSync(environmentPath)
    ? readFileSync(environmentPath, 'utf8')
    : ''
  const existingLines = existingContent
    .split(/\r?\n/)
    .filter(
      (line: string, index: number, lines: string[]): boolean =>
        index < lines.length - 1 || line.length > 0,
    )
  const updatedLines = getUpdatedEnvironmentLines(existingLines, updates)
  const temporaryPath = `${environmentPath}.tmp.${process.pid}`

  try {
    writeFileSync(temporaryPath, `${updatedLines.join('\n')}\n`, {
      encoding: 'utf8',
      mode: 0o600,
    })
    renameSync(temporaryPath, environmentPath)
    chmodSync(environmentPath, 0o600)
    return environmentPath
  } finally {
    if (existsSync(temporaryPath)) {
      unlinkSync(temporaryPath)
    }
  }
}
