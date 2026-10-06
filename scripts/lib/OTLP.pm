package OTLP;

# Shared OTLP JSON envelopes and HTTP transport.

use strict;
use warnings;

use Exporter qw(import);
use HTTP::Tiny;
use JSON::PP;
use Time::HiRes qw(time);

use ScreamingSnake;

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

# Signal discriminators for envelope and endpoint lookup.
use constant {
    SIGNAL_METRICS => 'metrics',
    SIGNAL_TRACES  => 'traces',
    SIGNAL_LOGS    => 'logs',
};

# OTLP wrapper, item, and endpoint names by signal.
my %SIGNAL_META = (
    metrics => { wrapper_key => 'resourceMetrics', items_key => 'metrics',    path => '/v1/metrics' },
    traces  => { wrapper_key => 'resourceSpans',   items_key => 'spans',      path => '/v1/traces'  },
    logs    => { wrapper_key => 'resourceLogs',    items_key => 'logRecords', path => '/v1/logs'    },
);

my $JSON = JSON::PP->new->utf8->canonical(0)->allow_nonref(1);

# Decimal strings preserve OTLP nanosecond timestamps in JSON.
sub now_ns {
    return sprintf '%.0f', time() * 1_000_000_000;
}

# Standard seed resource attributes plus caller-supplied attributes.
sub resource_attrs {
    my ($service, %extra) = @_;
    my @base = (
        attr('service.name',           $service),
        attr('service.version',        $extra{version}     // '1.0.0'),
        attr('deployment.environment', $extra{environment} // 'production'),
    );
    my @rest = map  { attr($_ => $extra{$_}) }
               grep { $_ ne 'version' && $_ ne 'environment' }
               keys %extra;
    return [ @base, @rest ];
}

# Scope names are "${service}.${kind}".
sub scope_for {
    my ($service, $kind) = @_;
    return { name => "$service.$kind", version => '0.1.0' };
}

# Explicit type overrides inference. Otherwise:
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
        # OTLP JSON encodes int64 values as decimal strings.
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

sub envelope {
    my ($signal, $resource_attrs, $scope, $items) = @_;
    my $meta = $SIGNAL_META{$signal}
        or die "OTLP::envelope: unknown signal '$signal'";
    # OTLP uses a signal-specific scope wrapper.
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

# Return (HTTP status, error), using status 0 when no response arrives.
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

THE_END();
