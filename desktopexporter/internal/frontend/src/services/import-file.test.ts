import { describe, expect, it } from 'vitest'
import { detectImportFile } from './import-file'

const envelope =
  '{"evalId":"run","config":{},"results":{"version":3,"results":[]}}'

describe('shared file framing and dispatch', () => {
  it('detects content independently of extension and preserves JSONL envelope boundaries', async () => {
    expect(await detectImportFile(new File([envelope], 'trace.bin'))).toBe(
      'eval'
    )
    expect(await detectImportFile(new Blob([envelope + '\n' + envelope]))).toBe(
      'eval'
    )
    expect(
      await detectImportFile(
        new Blob(['{"resourceSpans":[]}\n{"resourceLogs":[]}'])
      )
    ).toBe('otlp')
  })

  it.each([
    envelope + '\n{"late":',
    envelope + '\n{"resourceLogs":[]}',
    '{"evalId":"run","resourceLogs":[]}',
    '{"id":"result-only","success":true}',
    '{"manifest":{},"pilot":{}}',
    '[]',
  ])(
    'rejects malformed, mixed or context-free input before dispatch',
    async input => {
      await expect(detectImportFile(new Blob([input]))).rejects.toThrow()
    }
  )

  it('rejects invalid UTF-8 and respects cancellation', async () => {
    await expect(
      detectImportFile(
        new Blob([new Uint8Array([123, 34, 255, 34, 58, 49, 125])])
      )
    ).rejects.toThrow()
    const controller = new AbortController()
    controller.abort()
    await expect(
      detectImportFile(new Blob([envelope]), controller.signal)
    ).rejects.toThrow()
  })
})
