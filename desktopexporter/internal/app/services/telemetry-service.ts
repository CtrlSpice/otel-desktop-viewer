import { RpcClient } from 'jsonrpc-ts';
import { TelemetryService } from '../types/jsonrpc-service';

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
    console.error('Debug: JSON-RPC error:', response.data.error);
    throw new Error(`JSON-RPC Error: ${response.data.error.message}`);
  }

  return response.data.result;
}

// Export typed methods for each RPC call
export const telemetryAPI = {
  // Trace methods
  getTraceSummaries: () => callRPC('getTraceSummaries', undefined),
  getTraceByID: (traceID: string) => callRPC('getTraceByID', [traceID]),
  clearTraces: () => callRPC('clearTraces', undefined),
  
  // Log methods
  getLogs: () => callRPC('getLogs', undefined),
  getLogByID: (logID: string) => callRPC('getLogByID', [logID]),
  getLogsByTraceID: (traceID: string) => callRPC('getLogsByTraceID', [traceID]),
  clearLogs: () => callRPC('clearLogs', undefined),
  
  // Metric methods
  getMetrics: () => callRPC('getMetrics', undefined) as Promise<any>,
  clearMetrics: () => callRPC('clearMetrics', undefined),
  
  // Utility methods
  loadSampleData: () => callRPC('loadSampleData', undefined),
};
