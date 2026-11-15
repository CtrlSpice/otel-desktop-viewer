import type { TraceColumn, LogColumn } from '@/types/api-types'
import type { JsonQueryNode } from '@/types/wire-types'
import type { DiscoveryOptions } from './discovery-types'
import type { QueryNode } from '@/search/model'
import { fetchArrow } from './arrow/arrow-fetch'
import type { ArrowFrame } from './arrow/arrow-data'
import { JsonRpcError } from './service-error'

type JsonRpcParamValue =
  | string
  | number
  | boolean
  | null
  | JsonQueryNode
  | SearchSort
  | readonly (string | number)[]
  | readonly LogColumn[]
  | readonly TraceColumn[]
  | (Omit<DiscoveryOptions, 'queries'> & {
      queries?: Record<
        'traces' | 'metrics' | 'logs',
        JsonQueryNode | false | null
      >
    })

type JsonRpcNamedParams = Record<string, JsonRpcParamValue | undefined>
type JsonRpcParams = JsonRpcNamedParams | readonly string[]

interface JsonRpcRequest {
  jsonrpc: '2.0'
  method: string
  params?: JsonRpcParams
  id: number
}

interface JsonRpcResponse {
  jsonrpc: '2.0'
  result?: unknown
  error?: {
    code: number
    message: string
  }
  id: number
}

export type SearchSort = {
  field: string
  direction: 'asc' | 'desc'
}

/** An absolute query bound in Unix nanoseconds, or null when unbounded. */
export type QueryTimeBound = bigint | null

export function serializeNanoseconds(bound: QueryTimeBound): string | null {
  return bound === null ? null : bound.toString()
}

/** Thrown when a superseded request is abandoned. */
export class RequestAbortedError extends Error {
  constructor() {
    super('Request aborted')
    this.name = 'RequestAbortedError'
  }
}

/** Returns whether a rejection represents an abandoned request. */
// oxlint-disable-next-line anti-slop/no-unknown-parameters -- Classifies arbitrary caught or rejected values by identity without assuming Error.
export function isAbortError(err: unknown): boolean {
  return (
    err instanceof RequestAbortedError ||
    (err instanceof DOMException && err.name === 'AbortError')
  )
}

// Omit unsupplied keys while preserving null and empty-array values.
export function named(params: JsonRpcNamedParams): JsonRpcNamedParams {
  return Object.fromEntries(
    Object.entries(params).filter(([, value]) => value !== undefined)
  )
}

// T asserts the trusted backend wire shape at compile time; it does not validate it.
// Aborting fetch cancels the server request context and its DuckDB query.
export async function callRPC<T>(
  method: string,
  params?: JsonRpcParams,
  signal?: AbortSignal
): Promise<T> {
  const request: JsonRpcRequest = {
    method,
    params,
    id: Math.floor(Math.random() * 1000000),
    jsonrpc: '2.0',
  }

  let response: Response
  try {
    response = await fetch('/rpc', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(request),
      signal,
    })
  } catch (err) {
    if (isAbortError(err)) throw new RequestAbortedError()
    throw err
  }

  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }

  const data: JsonRpcResponse = await response.json()

  if (data.jsonrpc !== request.jsonrpc) {
    throw new Error(`Invalid JSON-RPC version: ${String(data.jsonrpc)}`)
  }
  if (data.id !== request.id) {
    throw new Error('JSON-RPC response ID does not match request ID')
  }

  if (data.error) {
    throw new JsonRpcError(data.error.code, data.error.message)
  }

  // SAFETY: Each successful in-process Go RPC method owns the wire type chosen
  // by its call site; service response tests pin the transformed wire shapes.
  // This assertion records that trusted boundary and performs no validation.
  return data.result as T
}

export async function callArrow(
  method: string,
  params: JsonRpcNamedParams,
  signal?: AbortSignal
): Promise<ArrowFrame> {
  try {
    return await fetchArrow(method, JSON.stringify(params), signal)
  } catch (error) {
    if (signal?.aborted || isAbortError(error)) throw new RequestAbortedError()
    throw error
  }
}

// Helper function to convert frontend query tree to minimal backend format
export function convertQueryTreeForBackend(
  queryTree: QueryNode
): JsonQueryNode {
  if (queryTree.type === 'text') {
    return {
      id: queryTree.id,
      type: 'text',
      query: {
        fieldOperator: queryTree.query.operator.symbol,
        value: queryTree.query.value,
      },
    }
  }
  if (queryTree.type === 'condition') {
    return {
      id: queryTree.id,
      type: 'condition',
      query: {
        field: {
          owner: queryTree.query.field.owner,
          path: queryTree.query.field.path,
          type: queryTree.query.field.type,
        },
        fieldOperator: queryTree.query.operator.symbol,
        value: queryTree.query.value,
      },
    }
  } else {
    return {
      id: queryTree.id,
      type: 'group',
      group: {
        logicalOperator: queryTree.group.operator,
        children: queryTree.group.children.map(convertQueryTreeForBackend),
      },
    }
  }
}
