package metrics

// FlushInterval exposes flushIntervalMetrics to the external test package.
// See the note on spans.FlushInterval. This one counts *metrics*, not
// datapoints, so a batch sized against it is smaller than it looks.
const FlushInterval = flushIntervalMetrics
