package nested

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/searchindex"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

var ErrUnsupportedMetricType = errors.New("unsupported metric type")

func stableID(parts ...string) duckdb.UUID {
	h := sha256.New()
	var length [8]byte
	for _, p := range parts {
		binary.LittleEndian.PutUint64(length[:], uint64(len(p)))
		h.Write(length[:])
		h.Write([]byte(p))
	}
	var id duckdb.UUID
	copy(id[:], h.Sum(nil))
	return id
}

func MetricIdentity(m pmetric.Metric, r, s map[string]any) (duckdb.UUID, int32, bool) {
	var temporality int32
	var monotonic bool
	switch m.Type() {
	case pmetric.MetricTypeSum:
		temporality = int32(m.Sum().AggregationTemporality())
		monotonic = m.Sum().IsMonotonic()
	case pmetric.MetricTypeHistogram:
		temporality = int32(m.Histogram().AggregationTemporality())
	case pmetric.MetricTypeExponentialHistogram:
		temporality = int32(m.ExponentialHistogram().AggregationTemporality())
	}
	id := stableID(ingest.FormatUUID(r["payload_id"].(duckdb.UUID)), ingest.FormatUUID(s["id"].(duckdb.UUID)), m.Name(), m.Unit(), m.Type().String(), strconv.FormatInt(int64(temporality), 10), strconv.FormatBool(monotonic))
	return id, temporality, monotonic
}

func IngestMetrics(ctx context.Context, conn driver.Conn, m pmetric.Metrics) (ingest.Rejected, error) {
	count := 0
	skip := map[int]error{}
	for _, r := range m.ResourceMetrics().All() {
		for _, s := range r.ScopeMetrics().All() {
			for _, metric := range s.Metrics().All() {
				switch metric.Type() {
				case pmetric.MetricTypeGauge, pmetric.MetricTypeSum, pmetric.MetricTypeHistogram, pmetric.MetricTypeExponentialHistogram:
				default:
					skip[count] = fmt.Errorf("%w: %s", ErrUnsupportedMetricType, metric.Type())
				}
				count++
			}
		}
	}
	return ingest.BisectingWrite(ctx, count, skip, func(lo, hi int) error {
		return ingest.InTransaction(ctx, conn, func() (err error) {
			a := metricAppender(ctx, conn)
			defer a.Discard()
			var catalogue searchindex.Batch
			ordinal := 0
			for _, rm := range m.ResourceMetrics().All() {
				resource, err := ingest.ResourceInput(rm.Resource(), rm.SchemaUrl())
				if err != nil {
					return err
				}
				service := ""
				if v, ok := util.LastValue(rm.Resource().Attributes(), "service.name"); ok {
					service = v.AsString()
				}
				for _, sm := range rm.ScopeMetrics().All() {
					scope, err := ingest.ScopeInput(sm.Scope(), sm.SchemaUrl())
					if err != nil {
						return err
					}
					a.SetContext(resource, scope)
					for _, metric := range sm.Metrics().All() {
						current := ordinal
						ordinal++
						if _, bad := skip[current]; bad || current < lo || current >= hi {
							continue
						}
						if err := ctx.Err(); err != nil {
							return err
						}
						id, temporality, mono := MetricIdentity(metric, resource, scope)
						metadata, err := ingest.AttributeBytes(metric.Metadata())
						if err != nil {
							return err
						}
						b := a.Builder()
						b.Field(0).(*array.StringBuilder).Append(ingest.FormatUUID(id))
						b.Field(1).(*array.StringBuilder).Append(ingest.FormatUUID(resource["id"].(duckdb.UUID)))
						b.Field(2).(*array.StringBuilder).Append(ingest.FormatUUID(resource["payload_id"].(duckdb.UUID)))
						b.Field(3).(*array.StringBuilder).Append(ingest.FormatUUID(scope["id"].(duckdb.UUID)))
						for i, v := range []string{metric.Name(), metric.Description(), metric.Unit()} {
							b.Field(i + 4).(*array.StringBuilder).Append(v)
						}
						b.Field(7).(*array.BinaryBuilder).Append(metadata)
						b.Field(8).(*array.StringBuilder).Append(metric.Type().String())
						b.Field(9).(*array.Int32Builder).Append(temporality)
						b.Field(10).(*array.BooleanBuilder).Append(mono)
						b.Field(11).(*array.StringBuilder).Append(service)
						recordID := duckdb.UUID(uuid.New())
						b.Field(14).(*array.StringBuilder).Append(ingest.FormatUUID(recordID))
						points := b.Field(15).(*array.ListBuilder)
						points.Append(true)
						times := pointTimes{}
						writer := pointWriter{b: points.ValueBuilder().(*array.StructBuilder), metric: id, record: recordID, resource: resource, scope: scope, catalogue: &catalogue, times: &times}
						switch metric.Type() {
						case pmetric.MetricTypeGauge:
							for _, dp := range metric.Gauge().DataPoints().All() {
								if err := writer.number(dp); err != nil {
									return err
								}
							}
						case pmetric.MetricTypeSum:
							for _, dp := range metric.Sum().DataPoints().All() {
								if err := writer.number(dp); err != nil {
									return err
								}
							}
						case pmetric.MetricTypeHistogram:
							for _, dp := range metric.Histogram().DataPoints().All() {
								if err := writer.histogram(dp); err != nil {
									return err
								}
							}
						case pmetric.MetricTypeExponentialHistogram:
							for _, dp := range metric.ExponentialHistogram().DataPoints().All() {
								if err := writer.exponential(dp); err != nil {
									return err
								}
							}
						}
						if times.count == 0 {
							b.Field(16).AppendNull()
							b.Field(17).AppendNull()
						} else {
							b.Field(16).(*array.Uint64Builder).Append(times.min)
							b.Field(17).(*array.Uint64Builder).Append(times.max)
						}
						a.FinishParent()
						if err := catalogueMetric(&catalogue, id, recordID, metadata, resource, scope, metric.Name(), metric.Description(), metric.Unit()); err != nil {
							return err
						}
					}
				}
			}
			err = a.Close()
			if err != nil {
				return err
			}
			if err := catalogue.Write(ctx, conn); err != nil {
				return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
			}
			return nil
		})
	})
}

type pointWriter struct {
	record          duckdb.UUID
	times           *pointTimes
	b               *array.StructBuilder
	metric          duckdb.UUID
	resource, scope map[string]any
	catalogue       *searchindex.Batch
}

type pointTimes struct {
	min, max uint64
	count    int
}

func (w pointWriter) null(i int)           { w.b.FieldBuilder(i).AppendNull() }
func (w pointWriter) u64(i int, v uint64)  { w.b.FieldBuilder(i).(*array.Uint64Builder).Append(v) }
func (w pointWriter) i32(i int, v int32)   { w.b.FieldBuilder(i).(*array.Int32Builder).Append(v) }
func (w pointWriter) f64(i int, v float64) { w.b.FieldBuilder(i).(*array.Float64Builder).Append(v) }
func (w pointWriter) optional(i int, has bool, v float64) {
	if has {
		w.f64(i, v)
	} else {
		w.null(i)
	}
}
func (w pointWriter) list(i int, v []uint64) {
	b := w.b.FieldBuilder(i).(*array.ListBuilder)
	b.Append(true)
	b.ValueBuilder().(*array.Uint64Builder).AppendValues(v, nil)
}
func (w pointWriter) prefix(t, start pcommon.Timestamp, flags pmetric.DataPointFlags) duckdb.UUID {
	v := uint64(t)
	if w.times.count == 0 || v < w.times.min {
		w.times.min = v
	}
	if w.times.count == 0 || v > w.times.max {
		w.times.max = v
	}
	w.times.count++
	id := duckdb.UUID(uuid.New())
	w.b.Append(true)
	w.b.FieldBuilder(0).(*array.StringBuilder).Append(ingest.FormatUUID(id))
	w.u64(1, uint64(t))
	w.u64(2, uint64(start))
	w.b.FieldBuilder(3).(*array.Uint32Builder).Append(uint32(flags))
	return id
}
func (w pointWriter) suffix(id duckdb.UUID, attrs pcommon.Map, exemplars pmetric.ExemplarSlice) error {
	raw, err := ingest.AttributeBytes(attrs)
	if err != nil {
		return err
	}
	w.b.FieldBuilder(20).(*array.BinaryBuilder).Append(raw)
	ids, err := ingest.AttributeIdentities(attrs)
	if err != nil {
		return err
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = ingest.FormatUUID(id)
	}
	w.b.FieldBuilder(21).(*array.StringBuilder).Append(strings.Join(parts, ","))
	list := w.b.FieldBuilder(22).(*array.ListBuilder)
	list.Append(true)
	e := list.ValueBuilder().(*array.StructBuilder)
	for _, ex := range exemplars.All() {
		eid := duckdb.UUID(uuid.New())
		e.Append(true)
		e.FieldBuilder(0).(*array.StringBuilder).Append(ingest.FormatUUID(eid))
		e.FieldBuilder(1).(*array.Uint64Builder).Append(uint64(ex.Timestamp()))
		switch ex.ValueType() {
		case pmetric.ExemplarValueTypeDouble:
			e.FieldBuilder(2).(*array.Float64Builder).Append(ex.DoubleValue())
			e.FieldBuilder(3).AppendNull()
		case pmetric.ExemplarValueTypeInt:
			e.FieldBuilder(2).AppendNull()
			e.FieldBuilder(3).(*array.Int64Builder).Append(ex.IntValue())
		default:
			e.FieldBuilder(2).AppendNull()
			e.FieldBuilder(3).AppendNull()
		}
		if tid := ex.TraceID(); tid.IsEmpty() {
			e.FieldBuilder(4).AppendNull()
		} else {
			e.FieldBuilder(4).(*array.StringBuilder).Append(uuid.UUID(tid).String())
		}
		if sid := ex.SpanID(); sid.IsEmpty() {
			e.FieldBuilder(5).AppendNull()
		} else {
			e.FieldBuilder(5).(*array.Uint64Builder).Append(binary.BigEndian.Uint64(sid[:]))
		}
		filtered, err := ingest.AttributeBytes(ex.FilteredAttributes())
		if err != nil {
			return err
		}
		e.FieldBuilder(6).(*array.BinaryBuilder).Append(filtered)
		if err := w.catalogue.Exemplar(w.metric, id, eid, filtered); err != nil {
			return err
		}
	}
	return cataloguePoint(w.catalogue, w.metric, w.record, id, raw, w.resource, w.scope)
}
func (w pointWriter) number(dp pmetric.NumberDataPoint) error {
	id := w.prefix(dp.Timestamp(), dp.StartTimestamp(), dp.Flags())
	switch dp.ValueType() {
	case pmetric.NumberDataPointValueTypeDouble:
		w.f64(4, dp.DoubleValue())
		w.null(5)
	case pmetric.NumberDataPointValueTypeInt:
		w.null(4)
		w.b.FieldBuilder(5).(*array.Int64Builder).Append(dp.IntValue())
	default:
		w.null(4)
		w.null(5)
	}
	w.b.FieldBuilder(6).(*array.StringBuilder).Append(dp.ValueType().String())
	for i := 7; i < 20; i++ {
		w.null(i)
	}
	return w.suffix(id, dp.Attributes(), dp.Exemplars())
}
func (w pointWriter) histogram(dp pmetric.HistogramDataPoint) error {
	id := w.prefix(dp.Timestamp(), dp.StartTimestamp(), dp.Flags())
	for i := 4; i < 7; i++ {
		w.null(i)
	}
	w.u64(7, dp.Count())
	w.optional(8, dp.HasSum(), dp.Sum())
	w.optional(9, dp.HasMin(), dp.Min())
	w.optional(10, dp.HasMax(), dp.Max())
	w.list(11, dp.BucketCounts().AsRaw())
	bounds := w.b.FieldBuilder(12).(*array.ListBuilder)
	bounds.Append(true)
	bounds.ValueBuilder().(*array.Float64Builder).AppendValues(dp.ExplicitBounds().AsRaw(), nil)
	for i := 13; i < 20; i++ {
		w.null(i)
	}
	return w.suffix(id, dp.Attributes(), dp.Exemplars())
}
func (w pointWriter) exponential(dp pmetric.ExponentialHistogramDataPoint) error {
	id := w.prefix(dp.Timestamp(), dp.StartTimestamp(), dp.Flags())
	for i := 4; i < 7; i++ {
		w.null(i)
	}
	w.u64(7, dp.Count())
	w.optional(8, dp.HasSum(), dp.Sum())
	w.optional(9, dp.HasMin(), dp.Min())
	w.optional(10, dp.HasMax(), dp.Max())
	w.null(11)
	w.null(12)
	w.i32(13, dp.Scale())
	w.u64(14, dp.ZeroCount())
	w.f64(15, dp.ZeroThreshold())
	w.i32(16, dp.Positive().Offset())
	w.list(17, dp.Positive().BucketCounts().AsRaw())
	w.i32(18, dp.Negative().Offset())
	w.list(19, dp.Negative().BucketCounts().AsRaw())
	return w.suffix(id, dp.Attributes(), dp.Exemplars())
}
