import * as http from 'node:http'
import * as https from 'node:https'
import { isIP } from 'node:net'

export class FixtureError extends Error {}
// Only the complete-response deadline below; callers may map it to their own budget.
export class FixtureDeadline extends FixtureError {}
export type Reply = { status: number, body: string }

// No redirects or retries: a failed request can still have committed upstream writes.
export function request(
  url: URL, method: string, headers: Record<string, string>, timeout: number, body: string | Buffer = '',
  responseLimit = 65_536,
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
        if (size > responseLimit) {
          // Settle with a sanitized error before destruction emits aborted/error events.
          reject(new FixtureError(responseLimit === 65_536 ? 'API response exceeds 64 KiB' :
            `API response exceeds ${responseLimit} bytes`))
          res.destroy()
          req.destroy()
        } else chunks.push(chunk)
      })
      res.on('end', () => resolve({ status: res.statusCode ?? 0, body: Buffer.concat(chunks).toString('utf8') }))
      res.on('aborted', () => reject(new FixtureError('API response incomplete')))
      res.on('error', () => reject(new FixtureError('API response failed')))
    })
    req.on('error', () => reject(new FixtureError('API request failed')))
    // A complete-response deadline, not a socket inactivity timeout (trickles must not extend it).
    deadline = setTimeout(() => {
      reject(new FixtureDeadline('API request deadline exceeded'))
      response?.destroy()
      req.destroy()
    }, timeout)
    req.end(body)
  }).finally(() => clearTimeout(deadline))
}
