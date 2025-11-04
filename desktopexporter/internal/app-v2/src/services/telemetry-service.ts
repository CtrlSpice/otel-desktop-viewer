// Telemetry Service - JSON-RPC API client for OpenTelemetry Desktop Viewer

import type {
  TraceData,
  TraceSummary,
  LogData,
  MetricData,
  Exemplar,
  GaugeDataPoint,
  SumDataPoint,
  HistogramDataPoint,
  ExponentialHistogramDataPoint,
  DataPoints,
} from '@/types/api-types';
import { PreciseTimestamp } from '@/types/precise-timestamp';
import type { QueryNode } from '@/components/PageHeader/search/queryTree';
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
    console.error(`RPC call failed for method ${method}:`, error);
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
          startTime: PreciseTimestamp.fromJSON(json.rootSpan.startTime),
          endTime: PreciseTimestamp.fromJSON(json.rootSpan.endTime),
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
        startTime: PreciseTimestamp.fromJSON(spanNode.spanData.startTime),
        endTime: PreciseTimestamp.fromJSON(spanNode.spanData.endTime),
        events: spanNode.spanData.events
          ? spanNode.spanData.events.map((event: any) => ({
              ...event,
              timestamp:
                event.timestamp && event.timestamp !== undefined
                  ? PreciseTimestamp.fromJSON(event.timestamp)
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
    timestamp: PreciseTimestamp.fromJSON(log.timestamp),
    observedTimestamp: PreciseTimestamp.fromJSON(log.observedTimestamp),
  }));
}

function exemplarFromJSON(json: any): Exemplar {
  return {
    ...json,
    timestamp: PreciseTimestamp.fromJSON(json.timestamp),
  };
}

function gaugeDataPointFromJSON(json: any): GaugeDataPoint {
  return {
    ...json,
    timestamp: PreciseTimestamp.fromJSON(json.timestamp),
    startTime: PreciseTimestamp.fromJSON(json.startTimeUnixNano),
    exemplars: json.exemplars?.map(exemplarFromJSON),
  };
}

function sumDataPointFromJSON(json: any): SumDataPoint {
  return {
    ...json,
    timestamp: PreciseTimestamp.fromJSON(json.timestamp),
    startTime: PreciseTimestamp.fromJSON(json.startTimeUnixNano),
    exemplars: json.exemplars?.map(exemplarFromJSON),
  };
}

function histogramDataPointFromJSON(json: any): HistogramDataPoint {
  return {
    ...json,
    timestamp: PreciseTimestamp.fromJSON(json.timestamp),
    startTime: PreciseTimestamp.fromJSON(json.startTimeUnixNano),
    exemplars: json.exemplars?.map(exemplarFromJSON),
  };
}

function exponentialHistogramDataPointFromJSON(
  json: any
): ExponentialHistogramDataPoint {
  return {
    ...json,
    timestamp: PreciseTimestamp.fromJSON(json.timestamp),
    startTime: PreciseTimestamp.fromJSON(json.startTimeUnixNano),
    exemplars: json.exemplars?.map(exemplarFromJSON),
  };
}

function dataPointsFromJSON(json: any): DataPoints {
  const points = json.points.map((point: any) => {
    switch (json.type) {
      case 'Gauge':
        return gaugeDataPointFromJSON(point);
      case 'Sum':
        return sumDataPointFromJSON(point);
      case 'Histogram':
        return histogramDataPointFromJSON(point);
      case 'ExponentialHistogram':
        return exponentialHistogramDataPointFromJSON(point);
      default:
        return point; // For Empty type or unknown types
    }
  });

  return {
    type: json.type,
    points,
  };
}

function metricDataFromJSON(json: any): MetricData {
  return {
    ...json,
    dataPoints: dataPointsFromJSON(json.dataPoints),
    received: PreciseTimestamp.fromJSON(json.received),
  };
}

function metricsFromJSON(json: any): MetricData[] {
  return json.map(metricDataFromJSON);
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
    console.log('getTraceAttributes: calling with params:', params);
    const rawData = await callRPC('getTraceAttributes', params);
    console.log('getTraceAttributes: received rawData:', rawData);

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
    console.log('getTraceAttributes: converted to:', converted);
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

  getTraceByID: async (
    traceID: string,
    startTime: number,
    endTime: number,
    queryTree?: QueryNode
  ): Promise<TraceData> => {
    const params = queryTree
      ? [traceID, startTime, endTime, convertQueryTreeForBackend(queryTree)]
      : [traceID, startTime, endTime];
    const rawData = await callRPC('getTraceByID', params);
    return traceDataFromJSON(rawData);
  },

  clearTraces: () => callRPC('clearTraces', undefined),

  // Log methods
  getLogs: async (
    startTime: number,
    endTime: number,
    queryTree?: QueryNode
  ): Promise<LogData[]> => {
    const params = queryTree
      ? [startTime, endTime, convertQueryTreeForBackend(queryTree)]
      : [startTime, endTime];
    const rawData = await callRPC('getLogs', params);
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
    const params = queryTree
      ? [startTime, endTime, convertQueryTreeForBackend(queryTree)]
      : [startTime, endTime];
    const rawData = await callRPC('getMetrics', params);
    return metricsFromJSON(rawData);
  },

  clearMetrics: () => callRPC('clearMetrics', undefined),

  // Sample data management
  loadSampleData: () => callRPC('loadSampleData', undefined),
  checkSampleDataExists: () => callRPC('checkSampleDataExists', undefined),
  clearSampleData: () => callRPC('clearSampleData', undefined),
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