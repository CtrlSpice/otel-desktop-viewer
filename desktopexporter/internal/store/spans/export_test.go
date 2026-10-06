package spans

// FlushInterval exposes flushIntervalSpans to the external test package.
// Tests derive batch sizes from it so they continue to cross the boundary.
const FlushInterval = flushIntervalSpans
