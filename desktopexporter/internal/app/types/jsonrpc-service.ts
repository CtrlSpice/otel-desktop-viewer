import { TraceSummary, TraceData, LogData } from './api-types';

// JSON-RPC Service Interface
// This maps method names to their parameter types
export interface TelemetryService {
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

// Return types for better type safety
export type TelemetryServiceReturns = {
  getTraceSummaries: TraceSummary[];
  getTraceByID: TraceData;
  clearTraces: string;
  getLogs: LogData[];
  getLogByID: LogData;
  getLogsByTraceID: LogData[];
  clearLogs: string;
  getMetrics: any[]; // TODO: Define MetricData type
  clearMetrics: string;
  loadSampleData: string;
};
