import { RpcClient } from 'jsonrpc-ts';
import { 
  TraceData, 
  TraceSummary, 
  LogData, 
  MetricData,
  Exemplar,
  GaugeDataPoint,
  SumDataPoint,
  HistogramDataPoint,
  ExponentialHistogramDataPoint,
  DataPoints
} from '../types/api-types';
import { PreciseTimestamp } from '../types/precise-timestamp';

// JSON-RPC Service Interface
// This maps method names to their parameter types
interface TelemetryService {
  // Trace methods
  getTraceSummaries: undefined;
  getTraceByID: [string];
  clearTraces: undefined;
  
  // Log methods
  getLogs: undefined;
  getLogByID: [string];
  getLogsByTraceID: [string];
  clearLogs: undefined;
  
  // Metric methods
  getMetrics: undefined;
  clearMetrics: undefined;
  
  // Utility methods
  loadSampleData: undefined;
}

// Create the RPC client instance
const rpcClient = new RpcClient<TelemetryService>({
  url: '/rpc', // This will be relative to the current domain
});

// Helper function to make typed RPC calls
async function callRPC(method: string, params?: any): Promise<any> {
  const request = {
    method: method as keyof TelemetryService,
    params,
    id: Math.floor(Math.random() * 1000000), // Generate random ID
    jsonrpc: '2.0' as const,
  };
  
  const response = await rpcClient.makeRequest(request);

  if (response.data.error) {
    throw new Error(`JSON-RPC Error: ${response.data.error.message}`);
  }

  return response.data.result;
}

// Helper functions to deserialize timestamps (moved from api-types.ts)
function traceSummaryFromJSON(json: any): TraceSummary {
  return {
    ...json,
    rootSpan: json.rootSpan ? {
      ...json.rootSpan,
      startTime: PreciseTimestamp.fromJSON(json.rootSpan.startTime),
      endTime: PreciseTimestamp.fromJSON(json.rootSpan.endTime)
    } : undefined
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
        events: spanNode.spanData.events ? spanNode.spanData.events.map((event: any) => ({
          ...event,
          timestamp: event.timestamp && event.timestamp !== undefined ? PreciseTimestamp.fromJSON(event.timestamp) : undefined
        })) : [],
        links: spanNode.spanData.links || []
      },
      depth: spanNode.depth
    }))
  };
}

function logsFromJSON(json: any): LogData[] {
  return json.map((log: any) => ({
    ...log,
    timestamp: PreciseTimestamp.fromJSON(log.timestamp),
    observedTimestamp: PreciseTimestamp.fromJSON(log.observedTimestamp)
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

function exponentialHistogramDataPointFromJSON(json: any): ExponentialHistogramDataPoint {
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

// Export typed methods for each RPC call with built-in conversion
export const telemetryAPI = {
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
  
  // Utility methods
  loadSampleData: () => callRPC('loadSampleData', undefined),
};
