package Traces;

# ============================================================================
# Traces.pm -- build OTLP trace payloads (spans, events, links) and POST them.
#
# This is the trace-side analogue of Metrics.pm: it knows how to shape
# spans into OTLP's resourceSpans/scopeSpans envelope and hand the result
# to OTLP::send_payload. A single trace can span multiple services, so
# send_trace accepts a list of *resource groups* (one per service) that
# all share the same trace id -- the multi-service waterfall the UI loves.
#
# Public surface:
#   - trace_id()                 -> 32 lowercase hex chars (OTLP trace id)
#   - span_id()                  -> 16 lowercase hex chars (OTLP span id)
#   - span(\%spec)               -> a span hashref
#   - event($name, $t_ns, \@attrs)        -> a span event hashref
#   - span_link($tid, $sid, \@attrs)      -> a span link hashref
#   - send_trace($endpoint, \@resource_groups) -> ($status, $err)
#
# A resource group is: { service => 'name', version => '1.0.0'?, spans => [...] }
#
# FP note: everything except send_trace is a pure data transform. ids are
# the one impurity (they read the RNG), kept tiny and obvious.
# ============================================================================

use strict;
use warnings;

use Exporter qw(import);

use ScreamingSnake;
use OTLP qw(
    SIGNAL_TRACES
    attr
    resource_attrs
    scope_for
    send_payload
);

our @EXPORT_OK = qw(
    trace_id
    span_id
    span
    event
    span_link
    send_trace
    s_to_ns
    KIND_INTERNAL
    KIND_SERVER
    KIND_CLIENT
    KIND_PRODUCER
    KIND_CONSUMER
    STATUS_UNSET
    STATUS_OK
    STATUS_ERROR
);
our %EXPORT_TAGS = ( all => \@EXPORT_OK );

# OTLP SpanKind enum.
use constant {
    KIND_INTERNAL => 1,
    KIND_SERVER   => 2,
    KIND_CLIENT   => 3,
    KIND_PRODUCER => 4,
    KIND_CONSUMER => 5,
};

# OTLP Status code enum.
use constant {
    STATUS_UNSET => 0,
    STATUS_OK    => 1,
    STATUS_ERROR => 2,
};

# Nanoseconds since epoch as a decimal string (JSON-safe 64-bit). Trace
# timings are integer ns; %d preserves Perl's 64-bit IV exactly, whereas
# %.0f would route through a double and lose the low digits (ns values
# are ~1.8e18, well past a double's 2^52 exact-integer range).
sub s_to_ns {
    my ($ns) = @_;
    return sprintf '%d', $ns;
}

# ----------------------------------------------------------------------------
# Id generation
# ----------------------------------------------------------------------------

# Random lowercase hex of the given byte length (2 hex chars per byte).
# Uses Perl's global rand, which seed.pl seeds once per process via
# srand, so a --seed run is reproducible.
sub _hex {
    my ($bytes) = @_;
    return join '', map { sprintf '%02x', int(rand 256) } 1 .. $bytes;
}

sub trace_id { _hex(16) }   # 16 bytes -> 32 hex chars
sub span_id  { _hex(8)  }   #  8 bytes -> 16 hex chars

# ----------------------------------------------------------------------------
# Span / event / link builders
# ----------------------------------------------------------------------------

# Build one span. Required: trace_id, span_id, name, start_ns, end_ns.
# Optional: parent_span_id, kind (default INTERNAL), status (default OK),
# attributes (arrayref), events (arrayref), links (arrayref).
sub span {
    my ($spec) = @_;
    my $s = {
        traceId           => $spec->{trace_id},
        spanId            => $spec->{span_id},
        name              => $spec->{name},
        kind              => ($spec->{kind} // KIND_INTERNAL) + 0,
        startTimeUnixNano => s_to_ns($spec->{start_ns}),
        endTimeUnixNano   => s_to_ns($spec->{end_ns}),
        status            => { code => ($spec->{status} // STATUS_OK) + 0 },
        attributes        => $spec->{attributes} // [],
    };
    $s->{parentSpanId} = $spec->{parent_span_id} if $spec->{parent_span_id};
    $s->{events}       = $spec->{events}         if $spec->{events};
    $s->{links}        = $spec->{links}          if $spec->{links};
    return $s;
}

# Build a span event: a named, timestamped marker with attributes.
sub event {
    my ($name, $t_ns, $attrs) = @_;
    return {
        timeUnixNano => s_to_ns($t_ns),
        name         => $name,
        attributes   => $attrs // [],
    };
}

# Build a span link: a pointer to another (trace_id, span_id) with
# attributes. The target need not exist in the store -- the UI renders
# the link either way (and now deep-links to /traces/<id>).
sub span_link {
    my ($tid, $sid, $attrs) = @_;
    return {
        traceId    => $tid,
        spanId     => $sid,
        attributes => $attrs // [],
    };
}

# ----------------------------------------------------------------------------
# Transport
# ----------------------------------------------------------------------------

# POST a (possibly multi-service) trace. Each resource group becomes one
# resourceSpans entry; they all share whatever trace id the caller stamped
# on the spans. We bypass OTLP::envelope here because envelope() models a
# single resource+scope, while a cross-service trace needs several.
sub send_trace {
    my ($endpoint, $resource_groups) = @_;
    my @resource_spans;
    for my $group (@$resource_groups) {
        my %extra;
        $extra{version} = $group->{version} if defined $group->{version};
        push @resource_spans, {
            resource => {
                attributes => resource_attrs($group->{service}, %extra),
            },
            scopeSpans => [{
                scope => scope_for($group->{service}, 'tracer'),
                spans => $group->{spans},
            }],
        };
    }
    return send_payload($endpoint, SIGNAL_TRACES, { resourceSpans => \@resource_spans });
}

THE_END();
