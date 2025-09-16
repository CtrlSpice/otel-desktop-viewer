// OpenTelemetry Semantic Conventions for attribute filtering
// Based on https://opentelemetry.io/docs/specs/semconv/

export interface AttributeSuggestion {
  name: string;
  category: string;
  description?: string;
}

export const SEMANTIC_CONVENTIONS: Record<string, AttributeSuggestion[]> = {
  http: [
    {
      name: 'http.method',
      category: 'HTTP',
      description: 'HTTP request method',
    },
    {
      name: 'http.url',
      category: 'HTTP',
      description: 'Full HTTP request URL',
    },
    {
      name: 'http.status_code',
      category: 'HTTP',
      description: 'HTTP response status code',
    },
    {
      name: 'http.user_agent',
      category: 'HTTP',
      description: 'Value of the HTTP User-Agent header',
    },
    {
      name: 'http.request_content_length',
      category: 'HTTP',
      description: 'Size of the request payload body in bytes',
    },
    {
      name: 'http.response_content_length',
      category: 'HTTP',
      description: 'Size of the response payload body in bytes',
    },
    {
      name: 'http.route',
      category: 'HTTP',
      description: 'The matched route (path template)',
    },
    {
      name: 'http.scheme',
      category: 'HTTP',
      description: 'The URI scheme identifying the used protocol',
    },
    {
      name: 'http.host',
      category: 'HTTP',
      description: 'Value of the HTTP Host header',
    },
    {
      name: 'http.target',
      category: 'HTTP',
      description: 'The full request target as passed in a HTTP request line',
    },
  ],
  database: [
    {
      name: 'db.system',
      category: 'Database',
      description: 'Database management system (DBMS) product',
    },
    { name: 'db.name', category: 'Database', description: 'Database name' },
    {
      name: 'db.operation',
      category: 'Database',
      description: 'Database operation being executed',
    },
    {
      name: 'db.sql.table',
      category: 'Database',
      description: 'Name of the primary table affected',
    },
    {
      name: 'db.connection_string',
      category: 'Database',
      description: 'Database connection string',
    },
    {
      name: 'db.statement',
      category: 'Database',
      description: 'Database statement being executed',
    },
    {
      name: 'db.user',
      category: 'Database',
      description: 'Username for accessing the database',
    },
  ],
  rpc: [
    {
      name: 'rpc.system',
      category: 'RPC',
      description: 'RPC system being used',
    },
    { name: 'rpc.service', category: 'RPC', description: 'Service name' },
    { name: 'rpc.method', category: 'RPC', description: 'RPC method name' },
    {
      name: 'rpc.grpc.status_code',
      category: 'RPC',
      description: 'gRPC status code',
    },
  ],
  messaging: [
    {
      name: 'messaging.system',
      category: 'Messaging',
      description: 'Messaging system being used',
    },
    {
      name: 'messaging.destination',
      category: 'Messaging',
      description: 'Destination name',
    },
    {
      name: 'messaging.operation',
      category: 'Messaging',
      description: 'Messaging operation',
    },
    {
      name: 'messaging.message_id',
      category: 'Messaging',
      description: 'Message identifier',
    },
    {
      name: 'messaging.conversation_id',
      category: 'Messaging',
      description: 'Conversation identifier',
    },
  ],
  error: [
    { name: 'error.name', category: 'Error', description: 'Error name' },
    { name: 'error.message', category: 'Error', description: 'Error message' },
    { name: 'error.stack', category: 'Error', description: 'Stack trace' },
  ],
  user: [
    { name: 'user.id', category: 'User', description: 'User identifier' },
    {
      name: 'user.name',
      category: 'User',
      description: 'Username or user display name',
    },
    { name: 'user.email', category: 'User', description: 'User email address' },
  ],
  system: [
    {
      name: 'service.name',
      category: 'System',
      description: 'Logical name of the service',
    },
    {
      name: 'service.version',
      category: 'System',
      description: 'Version of the service',
    },
    {
      name: 'service.instance.id',
      category: 'System',
      description: 'Instance ID of the service',
    },
    {
      name: 'deployment.environment',
      category: 'System',
      description: 'Deployment environment',
    },
    {
      name: 'telemetry.sdk.name',
      category: 'System',
      description: 'Telemetry SDK name',
    },
    {
      name: 'telemetry.sdk.version',
      category: 'System',
      description: 'Telemetry SDK version',
    },
  ],
};

// Flatten all suggestions for easy searching
export const ALL_ATTRIBUTE_SUGGESTIONS: AttributeSuggestion[] =
  Object.values(SEMANTIC_CONVENTIONS).flat();

// Get suggestions by category
export function getSuggestionsByCategory(
  category: string
): AttributeSuggestion[] {
  return SEMANTIC_CONVENTIONS[category] || [];
}

// Search suggestions by name
export function searchSuggestions(query: string): AttributeSuggestion[] {
  const lowercaseQuery = query.toLowerCase();
  return ALL_ATTRIBUTE_SUGGESTIONS.filter(suggestion =>
    suggestion.name.toLowerCase().includes(lowercaseQuery)
  );
}
