package Traces;

# Build and send OTLP trace payloads, including multi-service traces.

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

# %d preserves integer nanoseconds beyond double's exact 2^52 range.
sub s_to_ns {
    my ($ns) = @_;
    return sprintf '%d', $ns;
}

# seed.pl seeds the global RNG, making generated IDs reproducible.
sub _hex {
    my ($bytes) = @_;
    return join '', map { sprintf '%02x', int(rand 256) } 1 .. $bytes;
}

sub trace_id { _hex(16) }   # 16 bytes -> 32 hex chars
sub span_id  { _hex(8)  }   #  8 bytes -> 16 hex chars

# Required: trace_id, span_id, name, start_ns, and end_ns.
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

sub event {
    my ($name, $t_ns, $attrs) = @_;
    return {
        timeUnixNano => s_to_ns($t_ns),
        name         => $name,
        attributes   => $attrs // [],
    };
}

# Link targets need not exist in the store.
sub span_link {
    my ($tid, $sid, $attrs) = @_;
    return {
        traceId    => $tid,
        spanId     => $sid,
        attributes => $attrs // [],
    };
}

# Each resource group becomes one resourceSpans entry.
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
