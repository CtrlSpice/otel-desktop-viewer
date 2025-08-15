import React, { useState, useEffect } from "react";
import { Box, Heading, Button, Text, useColorModeValue } from "@chakra-ui/react";
import { telemetryAPI } from "../services/telemetry-service";
import { metricsFromJSON, MetricData } from "../types/api-types";

export default function MetricsView() {
  let [metrics, setMetrics] = useState<MetricData[] | null>(null);
  let [loading, setLoading] = useState(true);
  let [error, setError] = useState<string | null>(null);

  // Theme-aware colors
  let errorBg = useColorModeValue("red.100", "red.900");
  let errorColor = useColorModeValue("red.800", "red.200");
  let codeBg = useColorModeValue("gray.50", "gray.700");
  let codeColor = useColorModeValue("gray.800", "gray.100");

  let fetchMetrics = async () => {
    try {
      setLoading(true);
      setError(null);
      const rawData = await telemetryAPI.getMetrics();
      const data = metricsFromJSON(rawData);
      setMetrics(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch metrics");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchMetrics();
  }, []);

  return (
    <Box p={6} height="100vh" overflow="auto">
      <Box mb={4} display="flex" alignItems="center" gap={4}>
        <Heading size="lg">Metrics</Heading>
        <Button onClick={fetchMetrics} isLoading={loading} size="sm">
          Refresh
        </Button>
      </Box>

      {error && (
        <Box mb={4} p={4} bg={errorBg} color={errorColor} borderRadius="md">
          <Text fontWeight="bold">Error:</Text>
          <Text>{error}</Text>
        </Box>
      )}

      {loading && <Text>Loading metrics...</Text>}

      {!loading && !error && metrics && (
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
          {JSON.stringify(metrics, (key, value) => 
            typeof value === 'bigint' ? value.toString() : value, 2
          )}
        </Box>
      )}
    </Box>
  );
} 