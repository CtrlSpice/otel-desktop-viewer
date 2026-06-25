package Logs;

# ============================================================================
# Logs.pm -- build OTLP log records and POST them.
#
# The log-side analogue of Metrics.pm / Traces.pm. A log record is a
# flat thing (no tree), so this module is small: one builder for the
# record and one sender that wraps a batch of records for a single
# service into the resourceLogs/scopeLogs envelope.
#
# Public surface:
#   - log_record(\%spec)             -> a logRecord hashref
#   - send_logs($endpoint, $service, \@records) -> ($status, $err)
#
# Trace correlation: a record may carry trace_id/span_id. seed.pl wires
# a handful of records to *real* spans emitted by the trace run (via a
# small handoff file) so the UI's log -> trace deep link actually lands
# on a trace.
# ============================================================================

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

# ----------------------------------------------------------------------------
# Record builder
# ----------------------------------------------------------------------------

# Build one OTLP log record. Required: t_ns, severity_number,
# severity_text, body. Optional: observed_ns (defaults to t_ns + 500us
# to mirror the old shell seeder), trace_id, span_id, event_name,
# attributes (arrayref).
#
# Body is always emitted as stringValue -- matching the previous shell
# seeder, which sent JSON-shaped bodies as plain strings too (the viewer
# infers a string bodyType either way).
sub log_record {
    my ($spec) = @_;
    my $t_ns        = $spec->{t_ns};
    my $observed_ns = $spec->{observed_ns} // ($t_ns + 500_000);

    # %d (not %.0f): ns timestamps are ~1.8e18, past a double's 2^52
    # exact-integer range, so route them through Perl's 64-bit IV.
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

# ----------------------------------------------------------------------------
# Transport
# ----------------------------------------------------------------------------

# POST a batch of log records for a single service. Logs don't span
# services the way traces do, so the standard single-resource envelope
# fits and we reuse OTLP::envelope.
sub send_logs {
    my ($endpoint, $service, $records) = @_;
    my $resource = resource_attrs($service);
    my $scope    = scope_for($service, 'logger');
    my $payload  = envelope(SIGNAL_LOGS, $resource, $scope, $records);
    return send_payload($endpoint, SIGNAL_LOGS, $payload);
}

THE_END();
