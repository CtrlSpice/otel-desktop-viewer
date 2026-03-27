// Telemetry Service - JSON-RPC API client for OpenTelemetry Desktop Viewer

import type {
  TraceData,
  TraceSummary,
  LogData,
  MetricData,
  Stats,
  Exemplar,
  DataPoint,
} from '@/types/api-types';
import type { QueryNode } from '@/components/SignalHeader/search/queryTree';
import type {
  AttributeScope,
  FieldDefinition,
  FieldType,
} from '@/constants/fields';
import { getOperatorsForFieldType } from '@/constants/operators';

// JSON-RPC Client
interface JsonRpcRequest {
  jsonrpc: '2.0';
  method: string;
  params?: any;
  id: number;
}

interface JsonRpcResponse {
  jsonrpc: '2.0';
  result?: any;
  error?: {
    code: number;
    message: string;
  };
  id: number;
}

// Helper function to convert milliseconds to nanoseconds
function toNanoseconds(milliseconds: number): string {
  return milliseconds === 0 ? '0' : milliseconds.toString() + '000000';
}

// Helper function to make typed RPC calls
async function callRPC(method: string, params?: any): Promise<any> {
  const request: JsonRpcRequest = {
    method,
    params,
    id: Math.floor(Math.random() * 1000000),
    jsonrpc: '2.0',
  };

  try {
    const response = await fetch('/rpc', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(request),
    });

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    const data: JsonRpcResponse = await response.json();

    if (data.error) {
      throw new Error(`JSON-RPC Error: ${data.error.message}`);
    }

    return data.result;
  } catch (error) {
    throw error;
  }
}

// Data Transformation Functions

// Helper functions to deserialize timestamps
function traceSummaryFromJSON(json: any): TraceSummary {
  return {
    ...json,
    rootSpan: json.rootSpan
      ? {
          ...json.rootSpan,
          startTime: BigInt(json.rootSpan.startTime),
          endTime: BigInt(json.rootSpan.endTime),
        }
      : undefined,
  };
}

function traceSummariesFromJSON(json: any): TraceSummary[] {
  return json.map(traceSummaryFromJSON);
}

function traceDataFromJSON(json: any): TraceData {
  return {
    ...json,
    spans: json.spans.map((spanNode: any) => ({
      spanData: {
        ...spanNode.spanData,
        startTime: BigInt(spanNode.spanData.startTime),
        endTime: BigInt(spanNode.spanData.endTime),
        events: spanNode.spanData.events
          ? spanNode.spanData.events.map((event: any) => ({
              ...event,
              timestamp:
                event.timestamp && event.timestamp !== undefined
                  ? BigInt(event.timestamp)
                  : undefined,
            }))
          : [],
        links: spanNode.spanData.links || [],
      },
      depth: spanNode.depth,
    })),
  };
}

function logsFromJSON(json: any): LogData[] {
  return json.map((log: any) => ({
    ...log,
    timestamp: BigInt(log.timestamp),
    observedTimestamp: BigInt(log.observedTimestamp),
  }));
}

function exemplarFromJSON(json: any): Exemplar {
  return {
    ...json,
    timestamp: BigInt(json.timestamp),
  };
}

function dataPointFromJSON(json: any): DataPoint {
  return {
    ...json,
    timestamp: BigInt(json.timestamp),
    startTime: BigInt(json.startTime),
    exemplars: json.exemplars?.map(exemplarFromJSON) ?? [],
  };
}

function metricDataFromJSON(json: any): MetricData {
  return {
    ...json,
    datapoints: json.datapoints?.map(dataPointFromJSON) ?? [],
    received: BigInt(json.received),
  };
}

function metricsFromJSON(json: any): MetricData[] {
  return json.map(metricDataFromJSON);
}

function parseNullableBigInt(value: unknown): bigint | null {
  if (value === null || value === undefined) return null;
  if (typeof value === 'bigint') return value;
  if (typeof value === 'string' || typeof value === 'number')
    return BigInt(value);
  throw new Error(`Invalid bigint value: ${String(value)}`);
}

function statsFromJSON(json: any): Stats {
  return {
    traces: {
      ...json.traces,
      lastReceived: parseNullableBigInt(json.traces?.lastReceived),
    },
    logs: {
      ...json.logs,
      lastReceived: parseNullableBigInt(json.logs?.lastReceived),
    },
    metrics: {
      ...json.metrics,
      lastReceived: parseNullableBigInt(json.metrics?.lastReceived),
    },
  };
}

// API Methods

// Export typed methods for each RPC call with built-in conversion
export let telemetryAPI = {
  // Trace methods
  getTraceAttributes: async (
    startTime: number,
    endTime: number
  ): Promise<FieldDefinition[]> => {
    const startTimeNs = toNanoseconds(startTime);
    const endTimeNs = toNanoseconds(endTime);
    const params = [startTimeNs, endTimeNs];
    const rawData = await callRPC('getTraceAttributes', params);

    // Validate that we received an array
    if (!Array.isArray(rawData)) {
      console.warn(
        'getTraceAttributes: Expected array, got:',
        typeof rawData,
        rawData
      );
      return [];
    }

    // Convert backend attribute data to FieldDefinition objects
    const converted = convertAttributesToFieldDefinitions(rawData);
    return converted;
  },

  searchTraces: async (
    startTime: number,
    endTime: number,
    queryTree?: QueryNode
  ): Promise<TraceSummary[]> => {
    const startTimeNs = toNanoseconds(startTime);
    const endTimeNs = toNanoseconds(endTime);

    const params = queryTree
      ? [startTimeNs, endTimeNs, convertQueryTreeForBackend(queryTree)]
      : [startTimeNs, endTimeNs];
    const rawData = await callRPC('searchTraces', params);
    return traceSummariesFromJSON(rawData);
  },

  // TODO: Update when integrating search - will need to add searchTraceSpans endpoint
  // for filtering spans within a trace by time range and query
  getTraceByID: async (traceID: string): Promise<TraceData> => {
    const params = [traceID];
    const rawData = await callRPC('getTraceByID', params);
    return traceDataFromJSON(rawData);
  },

  clearTraces: () => callRPC('clearTraces', undefined),

  // Log methods
  searchLogs: async (
    startTime: number,
    endTime: number,
    queryTree?: QueryNode
  ): Promise<LogData[]> => {
    const startTimeNs = toNanoseconds(startTime);
    const endTimeNs = toNanoseconds(endTime);
    const params = queryTree
      ? [startTimeNs, endTimeNs, convertQueryTreeForBackend(queryTree)]
      : [startTimeNs, endTimeNs];
    const rawData = await callRPC('searchLogs', params);
    return logsFromJSON(rawData);
  },

  getLogByID: (logID: string) => callRPC('getLogByID', [logID]),
  getLogsByTraceID: (traceID: string) => callRPC('getLogsByTraceID', [traceID]),
  clearLogs: () => callRPC('clearLogs', undefined),

  // Metric methods
  getMetrics: async (
    startTime: number,
    endTime: number,
    queryTree?: QueryNode
  ): Promise<MetricData[]> => {
    const startTimeNs = toNanoseconds(startTime);
    const endTimeNs = toNanoseconds(endTime);
    const params = queryTree
      ? [startTimeNs, endTimeNs, convertQueryTreeForBackend(queryTree)]
      : [startTimeNs, endTimeNs];
    const rawData = await callRPC('getMetrics', params);
    return metricsFromJSON(rawData);
  },

  clearMetrics: () => callRPC('clearMetrics', undefined),

  // Stats methods
  getStats: async (): Promise<Stats> => {
    const rawData = await callRPC('getStats');
    return statsFromJSON(rawData);
  },
};

// Helper function to convert frontend query tree to minimal backend format
function convertQueryTreeForBackend(queryTree: QueryNode): any {
  if (queryTree.type === 'condition') {
    return {
      id: queryTree.id,
      type: 'condition',
      query: {
        field: {
          ...(queryTree.query.field.searchScope !== 'global' && {
            name: queryTree.query.field.name,
          }),
          searchScope: queryTree.query.field.searchScope,
          ...(queryTree.query.field.searchScope === 'attribute' && {
            attributeScope: queryTree.query.field.attributeScope,
            type: queryTree.query.field.type,
          }),
        },
        fieldOperator: queryTree.query.operator.symbol,
        value: queryTree.query.value,
      },
    };
  } else {
    return {
      id: queryTree.id,
      type: 'group',
      group: {
        logicalOperator: queryTree.group.operator,
        children: queryTree.group.children.map(convertQueryTreeForBackend),
      },
    };
  }
}

// Helper function to convert backend attribute data to FieldDefinition objects
function convertAttributesToFieldDefinitions(
  attributes: { name: string; type: string; attributeScope: string }[]
): FieldDefinition[] {
  return attributes
    .filter(attr => attr && attr.name && attr.type && attr.attributeScope) // Filter out invalid entries
    .map(attr => ({
      name: attr.name,
      type: attr.type as FieldType,
      searchScope: 'attribute' as const,
      attributeScope: attr.attributeScope as AttributeScope,
      operators: getOperatorsForFieldType(attr.type as FieldType),
    }));
}
