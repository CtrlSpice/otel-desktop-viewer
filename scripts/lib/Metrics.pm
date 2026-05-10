package Metrics;

# ============================================================================
# Metrics.pm -- build OTLP metric payloads from sampled shapes.
#
# This module is the bridge between Shapes.pm (which produces a list
# of { t_s, value } pairs) and OTLP.pm (which posts an envelope to
# the collector). It knows the four metric kinds and how to lay out
# their datapoint structures.
#
# Public functions (all take a final %opts hash for cross-cutting
# options like temporality, attributes, exemplars):
#   - gauge_metric(\%spec)       -> $metric_hash
#   - sum_metric(\%spec)         -> $metric_hash
#   - histogram_metric(\%spec)   -> $metric_hash
#   - exphist_metric(\%spec)     -> $metric_hash
#   - send_metric($endpoint, $service, $metric, %opts)
#                                -> ($status, $err)
#
# Spec shapes are described above each constructor.
# ============================================================================

use strict;
use warnings;

use Exporter qw(import);
use List::Util qw(sum0 min max);

use OTLP qw(
    SIGNAL_METRICS
    attr
    envelope
    resource_attrs
    scope_for
    send_payload
);

our @EXPORT_OK = qw(
    gauge_metric
    sum_metric
    histogram_metric
    exphist_metric
    send_metric
    AGG_DELTA
    AGG_CUMULATIVE
    AGG_UNSPECIFIED
    bucket_dist
    s_to_ns
);
our %EXPORT_TAGS = ( all => \@EXPORT_OK );

# Aggregation temporality enum from OTLP. UNSPECIFIED is 0 -- the
# spec says it MUST NOT be used, which is exactly why we send it
# (to exercise the "fun error" path).
use constant {
    AGG_UNSPECIFIED => 0,
    AGG_DELTA       => 1,
    AGG_CUMULATIVE  => 2,
};

# Time conversion: shapes work in seconds, OTLP wants nanoseconds.
# Returned as a string so the caller can drop it straight into JSON
# without sprintf-rounding.
sub s_to_ns {
    my ($s) = @_;
    return sprintf '%.0f', $s * 1_000_000_000;
}

# ----------------------------------------------------------------------------
# Datapoint builders (one per kind)
# ----------------------------------------------------------------------------

# Gauge / Sum share a datapoint shape: a single asDouble value at a
# single timestamp. Sum adds isMonotonic + temporality at the metric
# level (not the datapoint level).
sub _number_datapoint {
    my (%p) = @_;
    my $dp = {
        startTimeUnixNano => s_to_ns($p{start_s}),
        timeUnixNano      => s_to_ns($p{t_s}),
        asDouble          => $p{value} + 0,
        attributes        => $p{attributes} // [],
    };
    if ($p{exemplar_trace_id} && $p{exemplar_span_id}) {
        $dp->{exemplars} = [{
            timeUnixNano       => s_to_ns($p{t_s}),
            asDouble           => $p{value} + 0,
            traceId            => $p{exemplar_trace_id},
            spanId             => $p{exemplar_span_id},
            filteredAttributes => [],
        }];
    }
    return $dp;
}

# Explicit-bound histogram. Caller hands us a value (the "centre" of
# the distribution at that timestamp) and a fan-out of sample weights;
# we deal them into the explicit buckets.
#
# We approximate a sample distribution by treating the shape value
# as the *mean* and synthesizing bucketCounts via a triangle around
# that mean. Cheaper than running real sampling; visually convincing.
sub _histogram_datapoint {
    my (%p) = @_;
    my $bounds      = $p{bounds};
    my $center      = $p{value};
    my $sample_count = $p{sample_count} // 100;
    my $spread      = $p{spread} // 0.5;   # std-dev as fraction of center

    # Triangle distribution: weight[i] = max(0, 1 - |bucket_mid - center| / width).
    # We score each bucket (including the +inf overflow) and normalise.
    my @counts = (0) x (scalar(@$bounds) + 1);
    my $width = $center * $spread;
    $width = 1e-9 if $width <= 0;

    # Bucket midpoints: for buckets 0..N-1 use (lower+upper)/2; for the
    # overflow bucket (index N), use 1.5 * last_bound as a stand-in.
    my @midpoints;
    push @midpoints, $bounds->[0] / 2;   # bucket 0 covers (-inf, bounds[0]]
    for (my $i = 1; $i < @$bounds; $i++) {
        push @midpoints, ($bounds->[$i - 1] + $bounds->[$i]) / 2;
    }
    push @midpoints, $bounds->[-1] * 1.5;

    my @weights = map {
        my $d = abs($_ - $center);
        my $w = 1 - $d / (2 * $width);
        $w < 0 ? 0 : $w;
    } @midpoints;

    my $total_w = sum0 @weights;
    if ($total_w <= 0) {
        # Degenerate: dump everything into the bucket nearest center.
        my $best_i = 0; my $best_d = abs($midpoints[0] - $center);
        for (my $i = 1; $i < @midpoints; $i++) {
            my $d = abs($midpoints[$i] - $center);
            if ($d < $best_d) { $best_d = $d; $best_i = $i; }
        }
        $counts[$best_i] = $sample_count;
    } else {
        for (my $i = 0; $i < @weights; $i++) {
            $counts[$i] = int($sample_count * $weights[$i] / $total_w + 0.5);
        }
    }

    my $count = sum0 @counts;
    my $sum   = $count * $center;

    return {
        startTimeUnixNano => s_to_ns($p{start_s}),
        timeUnixNano      => s_to_ns($p{t_s}),
        count             => "$count",
        sum               => $sum + 0,
        min               => max(0, $center - 2 * $width) + 0,
        max               => ($center + 2 * $width) + 0,
        bucketCounts      => [ map { "$_" } @counts ],
        explicitBounds    => [ map { $_ + 0 } @$bounds ],
        attributes        => $p{attributes} // [],
    };
}

# Exponential histogram. Similar idea but the buckets have
# exponentially-growing widths controlled by `scale`. We pick an
# offset such that the center of the distribution lands roughly in
# the middle of a 9-bucket window, then triangle-distribute.
sub _exphist_datapoint {
    my (%p) = @_;
    my $center       = $p{value};
    my $scale        = $p{scale}        // 3;
    my $sample_count = $p{sample_count} // 100;
    my $spread       = $p{spread}       // 0.5;
    my $window       = $p{window}       // 9;   # bucket count

    # Bucket i covers [base^i, base^(i+1)) where base = 2^(2^-scale).
    my $base = 2 ** (2 ** -$scale);
    my $center_idx = int(log($center) / log($base));
    my $offset = $center_idx - int($window / 2);

    my @counts;
    for (my $i = 0; $i < $window; $i++) {
        my $idx = $offset + $i;
        my $lower = $base ** $idx;
        my $upper = $base ** ($idx + 1);
        my $mid   = ($lower + $upper) / 2;
        my $d     = abs($mid - $center);
        my $width = $center * $spread;
        $width = 1e-9 if $width <= 0;
        my $w = 1 - $d / (2 * $width);
        $w = 0 if $w < 0;
        push @counts, $w;
    }
    my $total_w = sum0 @counts;
    if ($total_w > 0) {
        @counts = map { int($sample_count * $_ / $total_w + 0.5) } @counts;
    } else {
        @counts = (0) x $window;
        $counts[int($window / 2)] = $sample_count;
    }

    my $count = sum0 @counts;
    my $sum   = $count * $center;
    my $width = $center * $spread;
    $width = 1e-9 if $width <= 0;

    return {
        startTimeUnixNano => s_to_ns($p{start_s}),
        timeUnixNano      => s_to_ns($p{t_s}),
        count             => "$count",
        sum               => $sum + 0,
        min               => max(0, $center - 2 * $width) + 0,
        max               => ($center + 2 * $width) + 0,
        scale             => $scale + 0,
        zeroCount         => "0",
        positive          => {
            offset       => $offset + 0,
            bucketCounts => [ map { "$_" } @counts ],
        },
        negative          => {
            offset       => 0,
            bucketCounts => [],
        },
        attributes        => $p{attributes} // [],
    };
}

# ----------------------------------------------------------------------------
# Metric constructors (produce the metric-level hash, not the envelope)
#
# Each takes a spec hash with at minimum:
#   name        => 'metric.name'
#   unit        => 's'
#   description => 'human description'
#   points      => \@points         (from Shapes::sample)
#   attributes  => \@otlp_attrs     (optional, applied to every dp)
#   step_s      => 60               (used to set startTimeUnixNano = t - step)
#
# Sum/Histogram/ExpHist also take:
#   temporality => AGG_DELTA / AGG_CUMULATIVE / AGG_UNSPECIFIED
# Sum additionally:
#   monotonic   => 1 / 0
# Histogram additionally:
#   bounds      => [explicit boundary list]
# ExpHist additionally:
#   scale       => integer (default 3)
# ----------------------------------------------------------------------------

sub _meta {
    my ($spec) = @_;
    return (
        name        => $spec->{name},
        description => $spec->{description} // '',
        unit        => $spec->{unit}        // '',
    );
}

sub _datapoints_from_points {
    my ($spec, $builder, %extra) = @_;
    my $step    = $spec->{step_s} // 60;
    my $attrs   = $spec->{attributes} // [];
    my @dps;
    for my $p (@{ $spec->{points} }) {
        push @dps, $builder->(
            t_s        => $p->{t_s},
            start_s    => $p->{t_s} - $step,
            value      => $p->{value},
            attributes => $attrs,
            %extra,
        );
    }
    return \@dps;
}

sub gauge_metric {
    my ($spec) = @_;
    return {
        _meta($spec),
        gauge => {
            dataPoints => _datapoints_from_points($spec, \&_number_datapoint),
        },
    };
}

sub sum_metric {
    my ($spec) = @_;
    return {
        _meta($spec),
        sum => {
            dataPoints             => _datapoints_from_points($spec, \&_number_datapoint),
            aggregationTemporality => ($spec->{temporality} // AGG_CUMULATIVE) + 0,
            isMonotonic            => $spec->{monotonic} ? \1 : \0,
        },
    };
}

sub histogram_metric {
    my ($spec) = @_;
    my $bounds = $spec->{bounds} // [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0];
    return {
        _meta($spec),
        histogram => {
            dataPoints => _datapoints_from_points($spec, \&_histogram_datapoint,
                bounds       => $bounds,
                sample_count => $spec->{sample_count} // 200,
                spread       => $spec->{spread}       // 0.4,
            ),
            aggregationTemporality => ($spec->{temporality} // AGG_DELTA) + 0,
        },
    };
}

sub exphist_metric {
    my ($spec) = @_;
    return {
        _meta($spec),
        exponentialHistogram => {
            dataPoints => _datapoints_from_points($spec, \&_exphist_datapoint,
                scale        => $spec->{scale}        // 3,
                sample_count => $spec->{sample_count} // 200,
                spread       => $spec->{spread}       // 0.4,
            ),
            aggregationTemporality => ($spec->{temporality} // AGG_DELTA) + 0,
        },
    };
}

# ----------------------------------------------------------------------------
# Sender
# ----------------------------------------------------------------------------

# Wraps a metric in the full OTLP envelope and POSTs it. The
# resource/scope are derived from $service.
sub send_metric {
    my ($endpoint, $service, $metric, %opts) = @_;
    my $resource = resource_attrs($service, %{ $opts{resource_extra} // {} });
    my $scope    = scope_for($service, 'meter');
    my $payload  = envelope(SIGNAL_METRICS, $resource, $scope, [$metric]);
    return send_payload($endpoint, SIGNAL_METRICS, $payload);
}

# ----------------------------------------------------------------------------
# Convenience: bucket_dist for callers who want to hand-craft distributions
# ----------------------------------------------------------------------------

# Build a triangle-shaped bucket count vector summing to ~$total,
# centered on bucket index $center, width $width buckets, across
# $n_buckets total. Useful in tests / one-off datapoints where you
# don't want a full shape.
sub bucket_dist {
    my (%p) = @_;
    my $n      = $p{n_buckets} // 11;
    my $center = $p{center}    // int($n / 2);
    my $width  = $p{width}     // 2;
    my $total  = $p{total}     // 100;
    my @raw    = map {
        my $d = abs($_ - $center);
        my $w = 1 - $d / $width;
        $w < 0 ? 0 : $w;
    } 0 .. ($n - 1);
    my $sum = sum0 @raw;
    return [(0) x $n] if $sum <= 0;
    return [ map { int($total * $_ / $sum + 0.5) } @raw ];
}

'So say we all!';
