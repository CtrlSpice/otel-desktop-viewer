package metrics

// MetricDataPoint represents the different types of metric data points
// This is how we get around the lack of algebraic data types in Go
type MetricDataPoint interface {
	isMetricDataPoint()
}

// Interface implementation methods
func (GaugeDataPoint) isMetricDataPoint()                {}
func (SumDataPoint) isMetricDataPoint()                  {}
func (HistogramDataPoint) isMetricDataPoint()            {}
func (ExponentialHistogramDataPoint) isMetricDataPoint() {}
