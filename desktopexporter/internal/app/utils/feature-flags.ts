// Feature flags for experimental or in-development features
export const FEATURE_FLAGS = {
  LOGS_VIEW: false, // Enable/disable the logs view
} as const;

// Type for feature flag names
export type FeatureFlag = keyof typeof FEATURE_FLAGS;

// Helper function to check if a feature is enabled
export function isFeatureEnabled(flag: FeatureFlag): boolean {
  return FEATURE_FLAGS[flag];
} 