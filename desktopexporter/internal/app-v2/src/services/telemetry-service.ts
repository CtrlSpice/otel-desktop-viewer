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
} from '../types/api-types';
import { PreciseTimestamp } from '../types/precise-timestamp';

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
  getTraceSummaries: async (): Promise<TraceSummary[]> => {
    const rawData = await callRPC('getTraceSummaries', undefined);
    return traceSummariesFromJSON(rawData);
  },

  getTraceByID: async (traceID: string): Promise<TraceData> => {
    const rawData = await callRPC('getTraceByID', [traceID]);
    return traceDataFromJSON(rawData);
  },

  clearTraces: () => callRPC('clearTraces', undefined),

  // Log methods
  getLogs: async (): Promise<LogData[]> => {
    const rawData = await callRPC('getLogs', undefined);
    return logsFromJSON(rawData);
  },

  getLogByID: (logID: string) => callRPC('getLogByID', [logID]),
  getLogsByTraceID: (traceID: string) => callRPC('getLogsByTraceID', [traceID]),
  clearLogs: () => callRPC('clearLogs', undefined),

  // Metric methods
  getMetrics: async (): Promise<MetricData[]> => {
    const rawData = await callRPC('getMetrics', undefined);
    return metricsFromJSON(rawData);
  },

  clearMetrics: () => callRPC('clearMetrics', undefined),

  // Sample data management
  loadSampleData: () => callRPC('loadSampleData', undefined),
  checkSampleDataExists: () => callRPC('checkSampleDataExists', undefined),
  clearSampleData: () => callRPC('clearSampleData', undefined),
};
