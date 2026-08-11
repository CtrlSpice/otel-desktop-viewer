package spans

// FlushInterval exposes flushIntervalSpans to the external test package.
//
// The flush-boundary test has to ingest more rows than the interval or it
// exercises nothing but the final Close. Hardcoding the number let it rot:
// the test said "51 // > flushIntervalSpans (50)" long after the constant was
// raised to 500 on measurement, so it kept passing while testing nothing.
// Deriving the batch size from the constant makes that failure impossible.
const FlushInterval = flushIntervalSpans
