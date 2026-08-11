package logs

// FlushInterval exposes flushIntervalLogs to the external test package.
// See the note on spans.FlushInterval: deriving the flush-boundary test's
// batch size from the constant is what stops it silently ceasing to cross
// the boundary when the constant moves.
const FlushInterval = flushIntervalLogs
