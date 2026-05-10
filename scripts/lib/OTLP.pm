package OTLP;

# ============================================================================
# OTLP.pm — shared OTLP HTTP transport and envelope helpers.
#
# Why this module exists:
#   The three signal types (metrics, traces, logs) all push to the same
#   collector via the same HTTP+JSON protocol with the same envelope
#   shape -- only the wrapper key changes ("resourceMetrics" vs
#   "resourceSpans" vs "resourceLogs") and the leaf collection name
#   ("metrics" vs "spans" vs "logRecords"). Putting the transport and
#   envelope here means the per-signal modules only worry about their
#   actual content (numeric models, span trees, severities), not about
#   how to wrap it for OTLP or how to POST it.
#
# What you get:
#   - now_ns()              : the current time in OTLP's nanosecond epoch
#   - resource_attrs(svc)   : standard "service.name + service.version
#                              + deployment.environment" attribute set
#   - scope_for($svc, $kind): scope { name, version } object for a service
#   - envelope($signal, $resource, $scope, \@items)
#                           : assembles the full JSON payload structure
#   - send_payload($endpoint, $signal, $payload_ref)
#                           : POSTs the JSON to /v1/{signal}; returns
#                              (http_status, error_message_or_undef)
#   - attr($key, $value [, $type])
#                           : builds a single OTLP attribute object,
#                              picking the right value type automatically
#
# FP notes for learners:
#   These are all pure-ish functions. send_payload is the only one with
#   side effects (the HTTP call). Everything else is a data
#   transformation: hash in, hash out. That's the FP shape -- the
#   "boring" core is composable hash-shuffling, the I/O is at the edges.
# ============================================================================

use strict;
use warnings;

use Exporter qw(import);
use HTTP::Tiny;
use JSON::PP;
use Time::HiRes qw(time);

our @EXPORT_OK = qw(
    now_ns
    resource_attrs
    scope_for
    envelope
    send_payload
    attr
    SIGNAL_METRICS
    SIGNAL_TRACES
    SIGNAL_LOGS
);

# Signal-type constants. Used as the discriminator for envelope() and
# send_payload() so callers don't have to memorise OTLP's slightly
# inconsistent naming (resourceMetrics + metrics vs resourceLogs +
# logRecords). Keeping these as named constants instead of bare strings
# means typos become compile-time errors via "use strict".
use constant {
    SIGNAL_METRICS => 'metrics',
    SIGNAL_TRACES  => 'traces',
    SIGNAL_LOGS    => 'logs',
};

# Per-signal metadata. The wrapper_key is the top-level array name in
# the OTLP payload ("resourceMetrics", etc.); the items_key is the leaf
# array name ("metrics", "spans", "logRecords"). Centralising this here
# means signal modules don't carry strings that have to agree with the
# protocol -- they just say `envelope(SIGNAL_METRICS, ...)`.
my %SIGNAL_META = (
    metrics => { wrapper_key => 'resourceMetrics', items_key => 'metrics',    path => '/v1/metrics' },
    traces  => { wrapper_key => 'resourceSpans',   items_key => 'spans',      path => '/v1/traces'  },
    logs    => { wrapper_key => 'resourceLogs',    items_key => 'logRecords', path => '/v1/logs'    },
);

# JSON encoder configured once and shared. JSON::PP is core Perl
# (no install needed), slower than JSON::XS but irrelevant for a
# seed script that runs hundreds of times, not millions.
my $JSON = JSON::PP->new->utf8->canonical(0)->allow_nonref(1);

# ----------------------------------------------------------------------------
# Time helpers
# ----------------------------------------------------------------------------

# OTLP timestamps are nanoseconds since Unix epoch, expressed as a
# decimal string (because JSON has no 64-bit int and JS clients would
# silently truncate). Time::HiRes::time gives us float seconds with
# microsecond precision; we multiply up and floor to int. Returned as a
# string so the caller can drop it straight into JSON without worrying
# about float printf rounding the bottom digits.
sub now_ns {
    return sprintf '%.0f', time() * 1_000_000_000;
}

# ----------------------------------------------------------------------------
# Identity (resource + scope) helpers
# ----------------------------------------------------------------------------

# Build the standard resource.attributes array for a given service.
# All seed-data services use the same shape (service.name +
# service.version + deployment.environment) so callers just hand in a
# service name and we expand it.
#
# FP note: this is a pure function. Same input -> same output, no
# side effects, no captured state. That makes it cheap to compose with
# other functions (e.g. envelope) and trivial to test in isolation.
sub resource_attrs {
    my ($service, %extra) = @_;
    my @base = (
        attr('service.name',           $service),
        attr('service.version',        $extra{version}     // '1.0.0'),
        attr('deployment.environment', $extra{environment} // 'production'),
    );
    # Allow the caller to tack on extra resource attrs (e.g. host.name
    # for a per-host gauge) without forcing them through a positional
    # interface. We splat them in via map so each %extra entry beyond
    # version/environment becomes another attr() call.
    my @rest = map  { attr($_ => $extra{$_}) }
               grep { $_ ne 'version' && $_ ne 'environment' }
               keys %extra;
    return [ @base, @rest ];
}

# Build the scope { name, version } object for a service. We name
# instrumentation scopes consistently as "${service}.${kind}" where
# kind is "meter" / "tracer" / "logger" -- matching the project's
# existing seed scripts so signal cards keep reading the same.
sub scope_for {
    my ($service, $kind) = @_;
    return { name => "$service.$kind", version => '0.1.0' };
}

# ----------------------------------------------------------------------------
# Attribute helper
# ----------------------------------------------------------------------------

# Build a single OTLP { key, value: { ... } } attribute. Picks the
# value variant from the Perl scalar's apparent type, with an optional
# explicit override for the corner cases (you want intValue but you've
# got an integer that happens to fit in a Perl float, etc.).
#
# Type detection rules (cheapest -> fanciest):
#   - If $type is given, use it.
#   - If $value is a JSON::PP::Boolean, emit boolValue.
#   - If $value looks like an integer string, emit intValue (as string,
#     because OTLP wants 64-bit ints stringified).
#   - If $value looks like a number, emit doubleValue.
#   - Otherwise, emit stringValue.
sub attr {
    my ($key, $value, $type) = @_;

    my $variant;
    if (defined $type) {
        $variant = $type;
    } elsif (ref($value) eq 'JSON::PP::Boolean') {
        $variant = 'boolValue';
    } elsif (defined $value && $value =~ /^-?\d+$/) {
        $variant = 'intValue';
        # OTLP expects 64-bit ints as strings; coerce now so JSON::PP
        # doesn't decide on its own.
        $value = "$value";
    } elsif (defined $value && $value =~ /^-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?$/) {
        $variant = 'doubleValue';
        $value = $value + 0;   # numify so JSON emits a number not a string
    } else {
        $variant = 'stringValue';
        $value = defined $value ? "$value" : '';
    }

    return { key => $key, value => { $variant => $value } };
}

# ----------------------------------------------------------------------------
# Envelope assembly
# ----------------------------------------------------------------------------

# Produce the full OTLP payload structure. The only place in this
# module that knows about wrapper_key / items_key / scope_key --
# callers just hand us (signal, resource_attrs, scope, items) and we
# shape it correctly.
#
# Returns a Perl hashref; encode it with JSON::PP yourself (or pass it
# to send_payload, which does that).
sub envelope {
    my ($signal, $resource_attrs, $scope, $items) = @_;
    my $meta = $SIGNAL_META{$signal}
        or die "OTLP::envelope: unknown signal '$signal'";
    # OTLP uses three different wrapper names at the scope level:
    # scopeMetrics / scopeSpans / scopeLogs. Plain conditional --
    # clearer than any clever string interpolation.
    my $scope_key =
        $signal eq 'metrics' ? 'scopeMetrics' :
        $signal eq 'traces'  ? 'scopeSpans'   :
                               'scopeLogs';
    return {
        $meta->{wrapper_key} => [{
            resource    => { attributes => $resource_attrs },
            $scope_key  => [{
                scope             => $scope,
                $meta->{items_key} => $items,
            }],
        }],
    };
}

# ----------------------------------------------------------------------------
# Transport
# ----------------------------------------------------------------------------

# POST a payload to the right OTLP endpoint for the given signal.
# Returns (http_status, error_or_undef). On a 2xx response,
# error_or_undef is undef. On any other outcome (network failure,
# 4xx/5xx), error_or_undef holds a short message; status will be the
# HTTP code if we got one, 0 if we didn't.
#
# The HTTP::Tiny instance is module-level so we reuse the connection
# across the dozens of POSTs a seed run will make.
my $HTTP = HTTP::Tiny->new(timeout => 10);

sub send_payload {
    my ($endpoint, $signal, $payload) = @_;
    my $meta = $SIGNAL_META{$signal}
        or die "OTLP::send_payload: unknown signal '$signal'";
    my $url  = $endpoint . $meta->{path};
    my $body = $JSON->encode($payload);
    my $res  = $HTTP->post($url, {
        headers => { 'Content-Type' => 'application/json' },
        content => $body,
    });
    if ($res->{success}) {
        return ($res->{status}, undef);
    }
    my $err = $res->{reason} // 'unknown error';
    if (length($res->{content} // '') < 200) {
        $err .= ': ' . $res->{content};
    }
    return ($res->{status} // 0, $err);
}

'Make it so!';
