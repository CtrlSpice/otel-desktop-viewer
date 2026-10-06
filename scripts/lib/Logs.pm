package Logs;

# Build and send OTLP log records with optional trace correlation.

use strict;
use warnings;

use Exporter qw(import);

use ScreamingSnake;
use OTLP qw(
    SIGNAL_LOGS
    attr
    envelope
    resource_attrs
    scope_for
    send_payload
);

our @EXPORT_OK = qw(
    log_record
    send_logs
);
our %EXPORT_TAGS = ( all => \@EXPORT_OK );

# observed_ns defaults to t_ns + 500us. Bodies use stringValue.
sub log_record {
    my ($spec) = @_;
    my $t_ns        = $spec->{t_ns};
    my $observed_ns = $spec->{observed_ns} // ($t_ns + 500_000);

    # %d preserves integer nanoseconds beyond double's exact 2^52 range.
    my $rec = {
        timeUnixNano         => sprintf('%d', $t_ns),
        observedTimeUnixNano => sprintf('%d', $observed_ns),
        severityNumber       => $spec->{severity_number} + 0,
        severityText         => $spec->{severity_text},
        body                 => { stringValue => "$spec->{body}" },
        attributes           => $spec->{attributes} // [],
    };
    $rec->{traceId}   = $spec->{trace_id}   if $spec->{trace_id};
    $rec->{spanId}    = $spec->{span_id}    if $spec->{span_id};
    $rec->{eventName} = $spec->{event_name} if defined $spec->{event_name}
        && length $spec->{event_name};
    return $rec;
}

# Log batches use one resource envelope per service.
sub send_logs {
    my ($endpoint, $service, $records) = @_;
    my $resource = resource_attrs($service);
    my $scope    = scope_for($service, 'logger');
    my $payload  = envelope(SIGNAL_LOGS, $resource, $scope, $records);
    return send_payload($endpoint, SIGNAL_LOGS, $payload);
}

THE_END();
