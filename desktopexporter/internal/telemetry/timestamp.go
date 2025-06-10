package telemetry

// PreciseTimestamp represents a timestamp split into milliseconds and nanoseconds for JSON serialization
type PreciseTimestamp struct {
	Milliseconds int64 `json:"milliseconds"`
	Nanoseconds  int32 `json:"nanoseconds"`
}

// NewPreciseTimestamp creates a new PreciseTimestamp from a nanosecond timestamp
func NewPreciseTimestamp(nanos int64) PreciseTimestamp {
	return PreciseTimestamp{
		Milliseconds: nanos / 1e6,
		Nanoseconds:  int32(nanos % 1e6),
	}
}