/**
 * Parses a positive integer from an environment record or returns defaultValue.
 */
export function parsePositiveIntegerEnv(
  env: Record<string, string | undefined>,
  name: string,
  defaultValue: number,
): number {
  const value = env[name]
  if (value === undefined || value.trim() === '') {
    return defaultValue
  }
  const parsed = Number(value)
  if (!Number.isInteger(parsed) || parsed <= 0) {
    throw new Error(
      `Environment variable ${name} must be a positive integer, got "${value}"`,
    )
  }
  return parsed
}
