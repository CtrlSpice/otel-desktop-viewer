import React, { useState, useEffect } from "react";
import { Box, Heading, Button, Text, useColorModeValue } from "@chakra-ui/react";
import { telemetryAPI } from "../services/telemetry-service";


export default function LogsView() {
  let [logs, setLogs] = useState<any>(null);
  let [loading, setLoading] = useState(true);
  let [error, setError] = useState<string | null>(null);

  // Theme-aware colors
  let errorBg = useColorModeValue("red.100", "red.900");
  let errorColor = useColorModeValue("red.800", "red.200");
  let codeBg = useColorModeValue("gray.50", "gray.700");
  let codeColor = useColorModeValue("gray.800", "gray.100");

  let fetchLogs = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await telemetryAPI.getLogs();
      setLogs(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch logs");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchLogs();
  }, []);

  return (
    <Box p={6} height="100vh" overflow="auto">
      <Box mb={4} display="flex" alignItems="center" gap={4}>
        <Heading size="lg">Logs</Heading>
        <Button onClick={fetchLogs} isLoading={loading} size="sm">
          Refresh
        </Button>
      </Box>

      {error && (
        <Box mb={4} p={4} bg={errorBg} color={errorColor} borderRadius="md">
          <Text fontWeight="bold">Error:</Text>
          <Text>{error}</Text>
        </Box>
      )}

      {loading && <Text>Loading logs...</Text>}

      {!loading && !error && logs && (
        <Box
          as="pre"
          p={4}
          bg={codeBg}
          color={codeColor}
          borderRadius="md"
          overflow="auto"
          fontSize="sm"
          fontFamily="mono"
          whiteSpace="pre-wrap"
        >
          {JSON.stringify(logs, (key, value) => 
            typeof value === 'bigint' ? value.toString() : value, 2
          )}
        </Box>
      )}
    </Box>
  );
} 