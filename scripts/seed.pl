#!/usr/bin/env perl

# seed.pl -- populate the OTel Desktop Viewer with synthetic telemetry.
#
# Usage:
#   ./scripts/seed.pl --metrics                          # seed metrics
#   ./scripts/seed.pl --traces                           # seed traces
#   ./scripts/seed.pl --logs                             # seed logs
#   ./scripts/seed.pl --all                              # everything
#   ./scripts/seed.pl --all --endpoint http://...        # custom endpoint
#   ./scripts/seed.pl --metrics --seed 7                 # deterministic
#
# --traces writes correlation IDs for --logs; stale or missing handoffs use
# random IDs.

use strict;
use warnings;
use FindBin qw($Bin);
use lib "$Bin/lib";

use Getopt::Long;
use File::Spec;
use JSON::PP;

use OTLP    qw(attr now_ns);
use Shapes  qw(:all);
use Metrics qw(:all);
use Traces  qw(:all);
use Logs    qw(:all);

my $endpoint = $ENV{OTLP_ENDPOINT} || 'http://localhost:4318';
my $seed     = 42;
my $do_metrics = 0;
my $do_traces  = 0;
my $do_logs    = 0;
my $do_all     = 0;

GetOptions(
    'endpoint=s' => \$endpoint,
    'seed=i'     => \$seed,
    'metrics'    => \$do_metrics,
    'traces'     => \$do_traces,
    'logs'       => \$do_logs,
    'all'        => \$do_all,
) or die "Bad options. Try --help.\n";

if ($do_all) { $do_metrics = $do_traces = $do_logs = 1 }
unless ($do_metrics || $do_traces || $do_logs) {
    die "Pick at least one of: --metrics --traces --logs --all\n";
}

# Seed trace IDs, log IDs, and correlation selection independently of metric data.
srand($seed);

# now_s anchors metric shapes; now_ns preserves trace and log offsets as int64.
my $now_s  = time;
my $now_ns = $now_s * 1_000_000_000;

# Integer factors keep timestamp arithmetic out of floating point.
use constant {
    NS_PER_MS   => 1_000_000,
    NS_PER_SEC  => 1_000_000_000,
    NS_PER_MIN  => 60 * 1_000_000_000,
    NS_PER_HOUR => 3600 * 1_000_000_000,
};

# Cross-process trace/log correlation handoff.
my $CORRELATION_FILE = File::Spec->catfile(File::Spec->tmpdir, 'otv-seed-correlation.json');

# Ignore handoffs whose traces may no longer be retained.
use constant CORRELATION_MAX_AGE => 600;

# Metric scenarios
# Each scenario returns ($service, $metric).

# Convert scenario-relative seconds to Unix epoch seconds.
sub _absolute {
    my ($end_s, $duration_s, @points) = @_;
    my $start = $end_s - $duration_s;
    return map { { t_s => $start + $_->{t_s}, value => $_->{value} } } @points;
}

sub _scenarios {
    my ($rng) = @_;

    return (
        # CPU diurnal gauge: 4h at 30s intervals.
        sub {
            my $duration = 4 * 3600;
            my $shape = clamp(noisy(
                compose(
                    constant(0.45),
                    diurnal({ amplitude => 0.18, period_s => 86400, phase_s => -2 * 3600 }),
                ), 0.06, $rng,
            ), 0, 1);
            my @pts = _absolute($now_s, $duration, sample($shape, 0, $duration, 30));
            return ('api-gateway', gauge_metric({
                name        => 'system.cpu.utilization',
                unit        => '1',
                description => 'CPU utilisation as a fraction of capacity',
                points      => \@pts,
                step_s      => 30,
                attributes  => [ attr('host.name', 'gw-01') ],
            }));
        },

        # Per-core gauge: 8 series with distinct phases and one incident.
        sub {
            my $duration = 4 * 3600;
            my $step     = 30;
            my @cores;
            for my $i (0 .. 7) {
                # Cores 0-1 have a heavier baseline.
                my $base = ($i < 2) ? 0.55 : 0.30;
                my @parts = (
                    constant($base),
                    diurnal({
                        amplitude => 0.12,
                        period_s  => 86400,
                        # Stagger peaks by core.
                        phase_s   => -2 * 3600 + $i * 600,
                    }),
                );
                # Core 3 has a mid-window spike.
                if ($i == 3) {
                    push @parts, incident({
                        baseline   => 0,
                        peak       => 0.35,
                        start_s    => 5400,
                        ramp_s     => 90,
                        hold_s     => 600,
                        recovery_s => 900,
                    });
                }
                my $shape = clamp(noisy(compose(@parts), 0.08, $rng), 0, 1);
                my @pts = _absolute($now_s, $duration, sample($shape, 0, $duration, $step));
                push @cores, {
                    attributes => [
                        attr('host.name', 'worker-01'),
                        attr('cpu.core',  "core-$i"),
                    ],
                    points => \@pts,
                };
            }
            return ('worker-pool', gauge_metrics({
                name        => 'system.cpu.utilization.per_core',
                unit        => '1',
                description => 'Per-core CPU utilisation on worker-01',
                step_s      => $step,
            }, \@cores));
        },

        # Memory creep gauge: 6h.
        sub {
            my $duration = 6 * 3600;
            my $shape = noisy(
                compose(
                    constant(450 * 1024 * 1024),
                    creep({ slope_per_s => (200 * 1024 * 1024) / (6 * 3600) }),
                    diurnal({ amplitude => 50 * 1024 * 1024, period_s => 3600 }),
                ), 0.03, $rng,
            );
            my @pts = _absolute($now_s, $duration, sample($shape, 0, $duration, 60));
            return ('api-gateway', gauge_metric({
                name        => 'process.runtime.jvm.memory.usage',
                unit        => 'By',
                description => 'JVM heap memory currently in use',
                points      => \@pts,
                step_s      => 60,
            }));
        },

        # Queue depth gauge: 2h with two incidents.
        sub {
            my $duration = 2 * 3600;
            my $shape = clamp(noisy(
                compose(
                    constant(40),
                    incident({ baseline => 0, peak => 250, start_s =>  900, ramp_s => 60, hold_s => 240, recovery_s => 600 }),
                    incident({ baseline => 0, peak => 180, start_s => 5100, ramp_s => 30, hold_s => 180, recovery_s => 480 }),
                ), 0.1, $rng,
            ), 0, undef);
            my @pts = _absolute($now_s, $duration, sample($shape, 0, $duration, 30));
            return ('notification-service', gauge_metric({
                name        => 'messaging.queue.depth',
                unit        => '{messages}',
                description => 'Number of messages waiting in the notification queue',
                points      => \@pts,
                step_s      => 30,
            }));
        },

        # Cumulative request count: method x route x status class (30 streams).
        sub {
            my $duration = 30 * 60;
            my $step     = 60;
            my @methods = (
                { name => 'GET',  base => 90, err_pct => 0.012 },
                { name => 'POST', base => 35, err_pct => 0.028 },
                { name => 'PUT',  base => 8,  err_pct => 0.045 },
            );
            my @routes = (
                { name => '/api/v2/orders',   weight => 1.3 },
                { name => '/api/v2/products', weight => 1.0 },
                { name => '/api/v2/users',    weight => 0.7 },
                { name => '/api/v2/checkout', weight => 0.5 },
                { name => '/api/v2/auth',     weight => 1.6 },
            );
            my @statuses = (
                { name => '2xx', share_of_total => 1.0 },   # 1 - err_pct applied below
                { name => '5xx', share_of_total => 1.0 },   # err_pct applied below
            );

            my @streams;
            for my $m (@methods) {
                for my $r (@routes) {
                    for my $s (@statuses) {
                        # Per-stream request rate.
                        my $share = $s->{name} eq '5xx' ? $m->{err_pct} : (1 - $m->{err_pct});
                        my $rps   = $m->{base} * $r->{weight} * $share;

                        my $rate = noisy(
                            compose(
                                constant($rps),
                                diurnal({
                                    amplitude => $rps * 0.25,
                                    period_s  => 86400,
                                    phase_s   => -3600 * ($r->{weight} - 1),
                                }),
                            ),
                            0.08,
                            $rng,
                        );
                        my @raw = sample($rate, 0, $duration, $step);

                        # Give each cumulative stream a distinct initial total.
                        my $running = int(1_000 + $rps * 600);
                        my @pts;
                        for my $p (@raw) {
                            $running += $p->{value} * $step;
                            push @pts, {
                                t_s   => $now_s - $duration + $p->{t_s},
                                value => $running,
                            };
                        }

                        push @streams, {
                            attributes => [
                                attr('http.method',       $m->{name}),
                                attr('http.route',        $r->{name}),
                                attr('http.status_class', $s->{name}),
                            ],
                            points => \@pts,
                        };
                    }
                }
            }

            return ('api-gateway', sum_metrics({
                name        => 'http.server.request.count',
                unit        => '{requests}',
                description => 'Total HTTP requests received, by method/route/status',
                step_s      => $step,
                temporality => AGG_CUMULATIVE,
                monotonic   => 1,
            }, \@streams));
        },

        # Delta error count: service x error class (12 streams), including spikes.
        sub {
            my $duration = 60 * 60;
            my $step     = 60;
            my @services = qw(api-gateway payment-service notification-service shipping-service);
            my @classes  = qw(5xx timeout dependency);   # error class

            # Two service/class pairs spike.
            my %spikes = (
                'payment-service|timeout'    => {
                    peak => 18, start_s => 1500, ramp_s => 60, hold_s => 240, recovery_s => 480,
                },
                'api-gateway|5xx'            => {
                    peak => 12, start_s => 2700, ramp_s => 90, hold_s => 180, recovery_s => 600,
                },
            );

            my @streams;
            for my $svc (@services) {
                for my $cls (@classes) {
                    # Baseline varies by error class and service.
                    my $base = ($cls eq '5xx'     ? 3
                              : $cls eq 'timeout' ? 1.5
                              :                     0.7);
                    $base *= ($svc eq 'payment-service'      ? 1.4
                             : $svc eq 'api-gateway'         ? 1.1
                             : $svc eq 'notification-service'? 0.8
                             :                                  0.6);

                    my @parts = (constant($base));
                    if (my $sp = $spikes{"$svc|$cls"}) {
                        push @parts, incident({
                            baseline   => 0,
                            peak       => $sp->{peak},
                            start_s    => $sp->{start_s},
                            ramp_s     => $sp->{ramp_s},
                            hold_s     => $sp->{hold_s},
                            recovery_s => $sp->{recovery_s},
                        });
                    }

                    my $shape = clamp(
                        noisy(compose(@parts), 0.25, $rng),
                        0, undef,
                    );
                    my @pts = _absolute($now_s, $duration, sample($shape, 0, $duration, $step));

                    push @streams, {
                        attributes => [
                            attr('service.name', $svc),
                            attr('error.class',  $cls),
                        ],
                        points => \@pts,
                    };
                }
            }

            return ('api-gateway', sum_metrics({
                name        => 'http.server.error.count',
                unit        => '{errors}',
                description => 'Server-side errors per interval, by service/class',
                step_s      => $step,
                temporality => AGG_DELTA,
                monotonic   => 1,
            }, \@streams));
        },

        # UNSPECIFIED sum with values that differ sharply under Delta and Cumulative.
        sub {
            my $duration = 30 * 60;
            my $shape = noisy(
                compose(
                    constant(14_000),
                    creep({ slope_per_s => 60 / 60 }),   # ~60 jobs per minute, monotonically
                ),
                0.02, $rng,
            );
            my @pts = _absolute($now_s, $duration, sample($shape, 0, $duration, 60));
            return ('legacy-batch', sum_metric({
                name        => 'jobs.processed.count',
                unit        => '{jobs}',
                description => 'Jobs processed (legacy meter, temporality not declared)',
                points      => \@pts,
                step_s      => 60,
                temporality => AGG_UNSPECIFIED,
                monotonic   => 1,
            }));
        },

        # Delta duration histogram: 3 routes over 90 minutes; /orders spikes.
        sub {
            my $duration = 90 * 60;
            my $step     = 60;
            my @routes = (
                {
                    route => '/api/v2/orders',
                    shape => clamp(noisy(
                        compose(
                            constant(0.08),
                            incident({ baseline => 0, peak => 0.45, start_s => 1800, ramp_s => 120, hold_s => 600, recovery_s => 1200 }),
                        ), 0.15, $rng,
                    ), 0.001, undef),
                },
                {
                    route => '/api/v2/products',
                    shape => clamp(noisy(constant(0.04), 0.1, $rng), 0.001, undef),
                },
                {
                    route => '/api/v2/users',
                    shape => clamp(noisy(
                        compose(
                            constant(0.06),
                            creep({ slope_per_s => 0.04 / $duration }),
                        ), 0.1, $rng,
                    ), 0.001, undef),
                },
            );
            my @all_points;
            for my $r (@routes) {
                my @pts = _absolute($now_s, $duration, sample($r->{shape}, 0, $duration, $step));
                push @all_points, map {
                    { %$_, _attrs => [
                        attr('http.method', 'GET'),
                        attr('http.route',  $r->{route}),
                    ] }
                } @pts;
            }
            # Each datapoint carries route-specific attributes.
            my @dps;
            for my $p (@all_points) {
                push @dps, Metrics::_histogram_datapoint(
                    t_s          => $p->{t_s},
                    start_s      => $p->{t_s} - $step,
                    value        => $p->{value},
                    attributes   => $p->{_attrs},
                    bounds       => [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0],
                    sample_count => 200,
                    spread       => 0.4,
                );
            }
            my $metric = {
                name        => 'http.server.request.duration',
                description => 'Duration of inbound HTTP requests',
                unit        => 's',
                histogram   => {
                    dataPoints             => \@dps,
                    aggregationTemporality => AGG_DELTA + 0,
                },
            };
            return ('api-gateway', $metric);
        },

        # Histogram with UNSPECIFIED temporality.
        sub {
            my $duration = 30 * 60;
            my $shape = clamp(noisy(constant(0.12), 0.1, $rng), 0.001, undef);
            my @pts = _absolute($now_s, $duration, sample($shape, 0, $duration, 60));
            return ('legacy-batch', histogram_metric({
                name        => 'job.duration',
                unit        => 's',
                description => 'Job duration (legacy meter, temporality not declared)',
                points      => \@pts,
                step_s      => 60,
                temporality => AGG_UNSPECIFIED,
                bounds      => [0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0],
            }));
        },

        # 48h sawtooth gauge for date-aware axes.
        sub {
            my $duration = 48 * 3600;
            my $shape = noisy(
                sawtooth({ amplitude => 80, baseline => 10, period_s => 12 * 3600 }),
                0.05, $rng,
            );
            my @pts = _absolute($now_s, $duration, sample($shape, 0, $duration, 60));
            return ('shipping-service', gauge_metric({
                name        => 'queue.dispatch.depth',
                unit        => '{shipments}',
                description => 'Pending shipments awaiting dispatch (resets on each batch flush)',
                points      => \@pts,
                step_s      => 60,
            }));
        },

        # Delta exponential histogram.
        sub {
            my $duration = 60 * 60;
            my $step     = 60;
            my $shape = clamp(noisy(
                compose(
                    constant(180),
                    incident({ baseline => 0, peak => 1200, start_s => 1500, ramp_s => 120, hold_s => 360, recovery_s => 900 }),
                ), 0.1, $rng,
            ), 5, undef);
            my @pts = _absolute($now_s, $duration, sample($shape, 0, $duration, $step));
            return ('payment-service', exphist_metric({
                name        => 'payment.processing.duration',
                unit        => 'ms',
                description => 'End-to-end payment processing time',
                points      => \@pts,
                step_s      => $step,
                scale       => 3,
                temporality => AGG_DELTA,
                attributes  => [
                    attr('payment.provider', 'stripe'),
                    attr('payment.method',   'card'),
                ],
            }));
        },

        # Exponential histogram with UNSPECIFIED temporality.
        sub {
            my $duration = 30 * 60;
            my $shape = clamp(noisy(constant(220), 0.1, $rng), 5, undef);
            my @pts = _absolute($now_s, $duration, sample($shape, 0, $duration, 60));
            return ('legacy-batch', exphist_metric({
                name        => 'rpc.client.duration',
                unit        => 'ms',
                description => 'RPC client duration (legacy meter, temporality not declared)',
                points      => \@pts,
                step_s      => 60,
                scale       => 3,
                temporality => AGG_UNSPECIFIED,
            }));
        },
    );
}

sub run_metrics {
    my $rng = make_rng($seed);
    print "Sending metrics to $endpoint/v1/metrics ...\n";
    my @scenarios = _scenarios($rng);
    my $i = 0;
    for my $scn (@scenarios) {
        $i++;
        my ($service, $metric) = $scn->();
        my ($status, $err) = send_metric($endpoint, $service, $metric);
        my $kind = (keys %{ { map { $_ => 1 } qw(gauge sum histogram exponentialHistogram) } })[0];
        # Find the metric kind for status output.
        for my $k (qw(gauge sum histogram exponentialHistogram)) {
            if (exists $metric->{$k}) { $kind = $k; last }
        }
        my $label = sprintf '%-22s %s', $service, $metric->{name};
        my $desc  = $metric->{description} // '';
        if (length($desc) > 52) { $desc = substr($desc, 0, 49) . '...' }
        if (defined $err) {
            printf "  [%2d] %-50s %-20s FAIL %s (%s)\n", $i, $label, $kind, $status, $err;
        } else {
            printf "  [%2d] %-50s %-20s %s\n", $i, $label, $kind, $status;
            printf "       %s\n", $desc if length $desc;
        }
    }
    print "Done. Sent ", scalar(@scenarios), " metrics.\n";
}

# Trace <-> log correlation handoff
# Best-effort array of {trace_id, span_id, service, name}.

sub _write_correlation {
    my ($entries) = @_;
    return unless @$entries;
    my $json = JSON::PP->new->utf8->canonical(1);
    open my $fh, '>', $CORRELATION_FILE or return;
    print {$fh} $json->encode($entries);
    close $fh;
}

sub _read_correlation {
    return [] unless -f $CORRELATION_FILE;
    # Its traces may no longer be retained.
    my $age = time - (stat _)[9];
    return [] if $age > CORRELATION_MAX_AGE;
    open my $fh, '<', $CORRELATION_FILE or return [];
    local $/;
    my $data = <$fh>;
    close $fh;
    my $parsed = eval { JSON::PP->new->utf8->decode($data) };
    return (ref $parsed eq 'ARRAY') ? $parsed : [];
}

# Trace assembly
#
# A trace is a list of "rows" (one per span). Each row is a hashref:
#   { svc, key, parent, name, kind, s, e, status, attrs, events, links }
# where s/e are ms offsets (may be fractional) from $base (default now),
# key/parent are *logical* names we map to generated span ids, events is
# [[name, off_ms, \@attrs], ...] and links is [[tid, sid, \@attrs], ...].
#
# An undefined parent name creates an orphan parent ID.

sub _assemble {
    my ($tid, $rows, %opt) = @_;
    my $base = $opt{base_ns} // $now_ns;

    # Generate IDs for spans and referenced orphan parents.
    my %sid;
    $sid{ $_->{key} } = span_id() for @$rows;
    for my $r (@$rows) {
        my $p = $r->{parent};
        $sid{$p} //= span_id() if defined $p && length $p;
    }

    my (%by_service, @order);
    for my $r (@$rows) {
        my $svc = $r->{svc};
        my @events = map { event($_->[0], $base + int($_->[1] * NS_PER_MS), $_->[2]) }
                     @{ $r->{events} // [] };
        my @links = map { span_link(@$_) } @{ $r->{links} // [] };
        my $attrs = $r->{attrs};
        $attrs = [ attr('service.layer', $svc) ] if !$attrs && $opt{add_layer};

        my $sp = span({
            trace_id => $tid,
            span_id  => $sid{ $r->{key} },
            (defined $r->{parent} && length $r->{parent}
                ? (parent_span_id => $sid{ $r->{parent} }) : ()),
            name     => $r->{name},
            kind     => $r->{kind} // KIND_INTERNAL,
            start_ns => $base + int($r->{s} * NS_PER_MS),
            end_ns   => $base + int($r->{e} * NS_PER_MS),
            status   => $r->{status} // STATUS_OK,
            attributes => $attrs // [],
            (@events ? (events => \@events) : ()),
            (@links  ? (links  => \@links)  : ()),
        });

        push @order, $svc unless $by_service{$svc};
        push @{ $by_service{$svc} }, $sp;
    }

    my $versions = $opt{versions} // {};
    return [ map {
        { service => $_,
          ($versions->{$_} ? (version => $versions->{$_}) : ()),
          spans => $by_service{$_} }
    } @order ];
}

# Root with children, events, links, and optional error status.
sub _simple_trace {
    my (%o) = @_;
    my $service   = $o{service};
    my $root_name = $o{root_name};
    my $status    = $o{status}    // STATUS_OK;
    my $children  = $o{children}  // 0;
    my $hours_ago = $o{hours_ago} // 0;
    my $dur_ms    = $o{dur_ms}    // 120;

    my $tid   = trace_id();
    my $root  = span_id();
    my $start = $now_ns - $hours_ago * NS_PER_HOUR;
    my $end   = $start + $dur_ms * NS_PER_MS;
    my $http  = $status == STATUS_ERROR ? 500 : 200;

    my @root_events = $status == STATUS_ERROR
        ? ( event('exception', $start, [
                attr('exception.type', 'RuntimeError'),
                attr('exception.message', "something went wrong in $root_name"),
            ]),
            event('http.response.flush', $start + 5 * NS_PER_MS, [
                attr('http.status_code', $http),
            ]) )
        : ( event('request.received', $start, [ attr('net.peer.ip', '10.0.1.2') ]),
            event('auth.verified', $start + 3 * NS_PER_MS, [ attr('auth.scheme', 'bearer') ]) );

    my @spans = span({
        trace_id => $tid, span_id => $root, name => $root_name,
        kind => KIND_SERVER, start_ns => $start, end_ns => $end, status => $status,
        attributes => [
            attr('http.method', 'GET'),
            attr('http.target', "/$root_name"),
            attr('http.status_code', $http),
        ],
        events => \@root_events,
        links  => [ span_link(trace_id(), span_id(), [
            attr('link.relationship', 'follows_from'),
            attr('messaging.system',  'kafka'),
        ]) ],
    });

    for my $c (1 .. $children) {
        my $cstart  = $start + $c * 5 * NS_PER_MS;
        my $cstatus = ($status == STATUS_ERROR && $c == 1) ? STATUS_ERROR : STATUS_OK;
        push @spans, span({
            trace_id => $tid, span_id => span_id(), parent_span_id => $root,
            name => "$root_name/child-$c", kind => KIND_CLIENT,
            start_ns => $cstart, end_ns => $cstart + 10 * NS_PER_MS, status => $cstatus,
            attributes => [ attr('child.index', $c) ],
            events => [ event('child.checkpoint', $cstart, [
                attr('child.index', $c), attr('checkpoint', 'after_dispatch'),
            ]) ],
            ($c == 2 ? (links => [ span_link(trace_id(), span_id(), [
                attr('peer.operation', 'synthetic_async_consumer'),
            ]) ]) : ()),
        });
    }

    my $label = "$service/$root_name" . ($status == STATUS_ERROR ? ' [ERROR]' : '');
    return {
        label     => $label,
        groups    => [ { service => $service, spans => \@spans } ],
        correlate => [ { trace_id => $tid, span_id => $root, service => $service, name => $root_name } ],
    };
}

# Screenshot fixture with fractional-millisecond offsets.
sub _sample_loadgenerator_trace {
    my $tid  = trace_id();
    my $base = $now_ns - 2 * NS_PER_HOUR;
    my $cart = sub {
        my ($target) = @_;
        [ attr('http.method', 'POST'),
          ($target =~ m{^http}
              ? attr('http.url',    $target)
              : attr('http.target', $target)),
          attr('http.status_code', 200) ];
    };
    my $groups = _assemble($tid, [
        { svc => 'sample-loadgenerator', key => 'root', name => 'sample-HTTP POST',
          kind => KIND_CLIENT, s => 0, e => 13.84, status => STATUS_UNSET,
          attrs => $cart->('http://frontend:8080/api/cart') },
        { svc => 'sample-loadgenerator', key => 'child', parent => 'root',
          name => 'sample-HTTP POST', kind => KIND_CLIENT, s => 1.2, e => 12.542,
          status => STATUS_UNSET, attrs => $cart->('http://frontend:8080/api/cart') },
        { svc => 'sample-frontend', key => 'frontend', parent => 'child',
          name => 'sample-HTTP POST', kind => KIND_SERVER, s => 2.6, e => 11.235,
          status => STATUS_UNSET, attrs => $cart->('/api/cart') },
    ], base_ns => $base);
    return {
        label  => 'sample-loadgenerator/HTTP POST /api/cart',
        groups => $groups,
    };
}

# Deep hierarchy fixture for waterfall depth.
sub _deep_hierarchy_trace {
    my $tid = trace_id();
    my $svc = 'deep-stack-service';
    my $groups = _assemble($tid, [
        { svc => $svc, key => 'shell', name => 'shell/checkout-flow', kind => KIND_SERVER,
          s => 0, e => 800,
          events => [[ 'checkout.session.start', 0, [ attr('cart.item_count', 3) ] ]],
          links  => [[ trace_id(), span_id(), [ attr('note', 'synthetic upstream marketing click') ] ]] },
        { svc => $svc, key => 'l1_http', parent => 'shell', name => 'HTTP GET /checkout',
          kind => KIND_CLIENT, s => 10, e => 600,
          events => [[ 'net.dns.lookup', 12, [ attr('net.host.name', 'api.internal') ] ]] },
        { svc => $svc, key => 'l2_db', parent => 'l1_http', name => 'db/checkout_snapshot',
          kind => KIND_CLIENT, s => 30, e => 500 },
        { svc => $svc, key => 'l3_db', parent => 'l2_db', name => 'db/line_items_subquery',
          kind => KIND_CLIENT, s => 50, e => 450,
          events => [[ 'db.stmt.complete', 55, [ attr('db.rows', 1842) ] ]] },
        { svc => $svc, key => 'l2_cache', parent => 'l1_http', name => 'cache/session_lookup',
          kind => KIND_CLIENT, s => 400, e => 580 },
        { svc => $svc, key => 'l1_worker', parent => 'shell', name => 'worker/dispatch_payment',
          kind => KIND_CLIENT, s => 20, e => 750 },
        { svc => $svc, key => 'l2_queue', parent => 'l1_worker', name => 'queue/consume_payment_job',
          kind => KIND_CLIENT, s => 100, e => 700 },
        { svc => $svc, key => 'l3_handler', parent => 'l2_queue', name => 'handler/process_payment',
          kind => KIND_CLIENT, s => 150, e => 650 },
        { svc => $svc, key => 'l4_grpc', parent => 'l3_handler', name => 'grpc/ChargeCard',
          kind => KIND_CLIENT, s => 200, e => 620,
          events => [[ 'rpc.metadata.received', 220, [ attr('rpc.grpc.status_code', 0) ] ]],
          links  => [[ trace_id(), span_id(), [ attr('rpc.system', 'grpc') ] ]] },
    ]);
    return { label => "$svc/checkout-flow (depth 5)", groups => $groups };
}

# Orphan fixture: parentSpanId values absent from the batch.
sub _orphan_spans_trace {
    my $tid = trace_id();
    my $svc = 'orphan-lab';
    my $groups = _assemble($tid, [
        { svc => $svc, key => 'root', name => 'ingest/partial-batch', kind => KIND_SERVER,
          s => 0, e => 400,
          events => [[ 'batch.parser.start', 0, [ attr('batch.bytes', 4096) ] ]],
          links  => [[ trace_id(), span_id(), [ attr('link.preset', 'prior_ingest_attempt') ] ]] },
        { svc => $svc, key => 'o1', parent => 'missing_a', name => 'orphan/missing_parent_A',
          kind => KIND_CLIENT, s => 20, e => 200,
          attrs  => [ attr('note', 'parentSpanId not in this export') ],
          events => [[ 'orphan.span.attached', 25, [ attr('synthetic', 'true') ] ]] },
        { svc => $svc, key => 'o2', parent => 'missing_b', name => 'orphan/missing_parent_B',
          kind => KIND_CLIENT, s => 40, e => 220,
          events => [[ 'work.unit.done', 45, [] ]],
          links  => [[ trace_id(), span_id(), [ attr('note', 'orphan still links outward') ] ]] },
        { svc => $svc, key => 'o3', parent => 'missing_a', name => 'orphan/sibling_same_missing_parent',
          kind => KIND_CLIENT, s => 60, e => 300,
          events => [[ 'dedupe.checkpoint', 70, [ attr('shard', 7) ] ]],
          links  => [[ trace_id(), span_id(), [] ]] },
    ], versions => { $svc => '0.0.1' });
    return { label => "$svc/partial-batch", groups => $groups };
}

# Duplicate-ID fixture: two rows share one _assemble key and span ID.
sub _duplicate_span_id_trace {
    my $tid = trace_id();
    my $svc = 'replay-lab';
    my $groups = _assemble($tid, [
        { svc => $svc, key => 'root', name => 'replay/session', kind => KIND_SERVER,
          s => 0, e => 400,
          attrs  => [ attr('replay.source', 'captured-session.otlp') ],
          events => [[ 'replay.start', 0, [ attr('replay.attempt', 2) ] ]] },
        { svc => $svc, key => 'shared', parent => 'root', name => 'replay/emit (first)',
          kind => KIND_CLIENT, s => 20, e => 180,
          attrs  => [ attr('note', 'kept: first span to claim this id') ],
          events => [[ 'span.emitted', 25, [] ]] },
        { svc => $svc, key => 'shared', parent => 'root', name => 'replay/emit (duplicate id)',
          kind => KIND_CLIENT, s => 200, e => 380,
          attrs  => [ attr('note', 'refused: same span id as the one above') ],
          events => [[ 'span.re-emitted', 205, [] ]] },
    ], versions => { $svc => '0.0.1' });
    return { label => "$svc/duplicate-span-id", groups => $groups };
}

# Orphan-subtree fixture: the head has a missing parent and two children.
sub _orphan_subtree_trace {
    my $tid = trace_id();
    my $svc = 'orphan-subtree-lab';
    my $groups = _assemble($tid, [
        { svc => $svc, key => 'head', parent => 'ghost', name => 'orphan/subtree_head',
          kind => KIND_CLIENT, s => 0, e => 350,
          attrs  => [ attr('edge', 'parent_span_id_not_in_batch') ],
          events => [[ 'subtree.head.bootstrap', 5, [ attr('children.expected', 2) ] ]],
          links  => [[ trace_id(), span_id(), [ attr('causal', 'scheduled_by_external') ] ]] },
        { svc => $svc, key => 'child_a', parent => 'head', name => 'orphan/subtree_head/child_a',
          kind => KIND_CLIENT, s => 30, e => 200,
          events => [[ 'child_a.phase1', 40, [] ],
                     [ 'child_a.phase2', 120, [ attr('rows', 12) ] ]] },
        { svc => $svc, key => 'child_b', parent => 'head', name => 'orphan/subtree_head/child_b',
          kind => KIND_CLIENT, s => 50, e => 280,
          events => [[ 'child_b.retry', 90, [ attr('attempt', 2) ] ]],
          links  => [[ trace_id(), span_id(), [ attr('downstream', 'synthetic') ] ]] },
    ], versions => { $svc => '0.0.1' });
    return { label => "$svc/subtree-under-missing-parent", groups => $groups };
}

# Multi-service order fixture with parallel branches and a payment error.
sub _large_multiservice_trace {
    my $tid = trace_id();
    my %versions = (
        'api-gateway'          => '2.4.1',
        'user-service'         => '1.8.0',
        'catalog-service'      => '3.1.2',
        'inventory-service'    => '1.2.0',
        'order-service'        => '2.0.3',
        'payment-service'      => '4.0.1',
        'notification-service' => '1.5.0',
        'shipping-service'     => '1.0.4',
    );
    my @rows = (
        { svc => 'api-gateway', key => 'gw_root', name => 'POST /api/v2/orders', kind => KIND_SERVER,
          s => 0, e => 1200,
          events => [[ 'request.received', 0, [ attr('http.method', 'POST'), attr('http.url', '/api/v2/orders') ] ]] },
        { svc => 'api-gateway', key => 'gw_auth',  parent => 'gw_root', name => 'middleware/authenticate', s => 2,  e => 18 },
        { svc => 'api-gateway', key => 'gw_rate',  parent => 'gw_root', name => 'middleware/rate-limit',  s => 18, e => 22 },
        { svc => 'api-gateway', key => 'gw_route', parent => 'gw_root', name => 'router/dispatch',        s => 22, e => 1180 },

        { svc => 'user-service', key => 'usr_validate',    parent => 'gw_auth',      name => 'user/validate-token',    s => 3, e => 16 },
        { svc => 'user-service', key => 'usr_profile',     parent => 'usr_validate', name => 'user/load-profile',      s => 5, e => 14 },
        { svc => 'user-service', key => 'usr_db_read',     parent => 'usr_profile',  name => 'db/SELECT users',        kind => KIND_CLIENT, s => 6, e => 10 },
        { svc => 'user-service', key => 'usr_cache_check', parent => 'usr_profile',  name => 'cache/profile-lookup',   kind => KIND_CLIENT, s => 6, e => 8 },
        { svc => 'user-service', key => 'usr_perms',       parent => 'usr_validate', name => 'user/check-permissions', s => 14, e => 16 },

        { svc => 'catalog-service', key => 'cat_list',   parent => 'gw_route',   name => 'catalog/resolve-items',    s => 25,  e => 280 },
        { svc => 'catalog-service', key => 'cat_db',     parent => 'cat_list',   name => 'db/SELECT products',       kind => KIND_CLIENT, s => 28, e => 120 },
        { svc => 'catalog-service', key => 'cat_cache',  parent => 'cat_list',   name => 'cache/product-details',    kind => KIND_CLIENT, s => 28, e => 45 },
        { svc => 'catalog-service', key => 'cat_enrich', parent => 'cat_list',   name => 'catalog/enrich-metadata',  s => 125, e => 270 },
        { svc => 'catalog-service', key => 'cat_img',    parent => 'cat_enrich', name => 'cdn/resolve-image-urls',   kind => KIND_CLIENT, s => 130, e => 200 },
        { svc => 'catalog-service', key => 'cat_price',  parent => 'cat_enrich', name => 'pricing/compute-discounts', s => 135, e => 260 },

        { svc => 'inventory-service', key => 'inv_reserve',  parent => 'gw_route',    name => 'inventory/reserve-stock', s => 285, e => 500 },
        { svc => 'inventory-service', key => 'inv_lock',     parent => 'inv_reserve', name => 'db/advisory-lock',        kind => KIND_CLIENT, s => 288, e => 310 },
        { svc => 'inventory-service', key => 'inv_db_write', parent => 'inv_reserve', name => 'db/UPDATE stock',         kind => KIND_CLIENT, s => 312, e => 420 },
        { svc => 'inventory-service', key => 'inv_db_read',  parent => 'inv_reserve', name => 'db/SELECT remaining',     kind => KIND_CLIENT, s => 422, e => 460 },
        { svc => 'inventory-service', key => 'inv_confirm',  parent => 'inv_reserve', name => 'inventory/confirm-hold',  s => 462, e => 495 },

        { svc => 'order-service', key => 'ord_create',    parent => 'gw_route',     name => 'order/create',            s => 505, e => 780 },
        { svc => 'order-service', key => 'ord_validate',  parent => 'ord_create',   name => 'order/validate-request',  s => 508, e => 530 },
        { svc => 'order-service', key => 'ord_db_insert', parent => 'ord_create',   name => 'db/INSERT orders',        kind => KIND_CLIENT, s => 532, e => 600 },
        { svc => 'order-service', key => 'ord_line1',     parent => 'ord_db_insert', name => 'db/INSERT line_items[0]', kind => KIND_CLIENT, s => 535, e => 555 },
        { svc => 'order-service', key => 'ord_line2',     parent => 'ord_db_insert', name => 'db/INSERT line_items[1]', kind => KIND_CLIENT, s => 556, e => 575 },
        { svc => 'order-service', key => 'ord_line3',     parent => 'ord_db_insert', name => 'db/INSERT line_items[2]', kind => KIND_CLIENT, s => 576, e => 595 },
        { svc => 'order-service', key => 'ord_total',     parent => 'ord_create',   name => 'order/compute-totals',    s => 602, e => 770 },

        { svc => 'payment-service', key => 'pay_charge',   parent => 'gw_route',   name => 'payment/charge',          s => 785, e => 1050 },
        { svc => 'payment-service', key => 'pay_fraud',    parent => 'pay_charge', name => 'payment/fraud-check',     s => 788, e => 880 },
        { svc => 'payment-service', key => 'pay_fraud_ml', parent => 'pay_fraud',  name => 'ml/score-transaction',    kind => KIND_CLIENT, s => 790, e => 870 },
        { svc => 'payment-service', key => 'pay_gateway',  parent => 'pay_charge', name => 'stripe/create-charge',    kind => KIND_CLIENT, s => 882, e => 1000, status => STATUS_ERROR,
          events => [[ 'exception', 950, [
              attr('exception.type', 'PaymentDeclinedError'),
              attr('exception.message', 'Card declined: insufficient funds') ] ]] },
        { svc => 'payment-service', key => 'pay_ledger',   parent => 'pay_charge', name => 'db/INSERT ledger_entry',  kind => KIND_CLIENT, s => 1002, e => 1030 },
        { svc => 'payment-service', key => 'pay_receipt',  parent => 'pay_charge', name => 'payment/generate-receipt', s => 1032, e => 1048 },

        { svc => 'notification-service', key => 'notif_email',   parent => 'gw_route',    name => 'notify/send-confirmation', s => 1055, e => 1170 },
        { svc => 'notification-service', key => 'notif_render',  parent => 'notif_email', name => 'template/render-email',    s => 1058, e => 1090 },
        { svc => 'notification-service', key => 'notif_smtp',    parent => 'notif_email', name => 'smtp/deliver',             kind => KIND_CLIENT, s => 1092, e => 1140 },
        { svc => 'notification-service', key => 'notif_push',    parent => 'notif_email', name => 'push/send-mobile',         kind => KIND_CLIENT, s => 1092, e => 1130 },
        { svc => 'notification-service', key => 'notif_webhook', parent => 'notif_email', name => 'webhook/post-order-event', kind => KIND_CLIENT, s => 1095, e => 1165 },

        { svc => 'shipping-service', key => 'ship_schedule', parent => 'gw_route',      name => 'shipping/schedule-pickup', s => 1055, e => 1175 },
        { svc => 'shipping-service', key => 'ship_carrier',  parent => 'ship_schedule', name => 'carrier/query-rates',      kind => KIND_CLIENT, s => 1060, e => 1120 },
        { svc => 'shipping-service', key => 'ship_label',    parent => 'ship_schedule', name => 'label/generate-pdf',       s => 1122, e => 1170 },
    );
    my $groups = _assemble($tid, \@rows, versions => \%versions, add_layer => 1);
    # Correlate logs with one entry span per service.
    my @correlate;
    for my $pick (
        [ 'api-gateway',          'POST /api/v2/orders'       ],
        [ 'order-service',        'order/create'              ],
        [ 'payment-service',      'payment/charge'            ],
        [ 'inventory-service',    'inventory/reserve-stock'   ],
        [ 'notification-service', 'notify/send-confirmation'  ],
        [ 'shipping-service',     'shipping/schedule-pickup'  ],
    ) {
        my $sid = _find_span_id($groups, $pick->[0], $pick->[1]);
        push @correlate, { trace_id => $tid, span_id => $sid,
                           service => $pick->[0], name => $pick->[1] } if $sid;
    }
    return {
        label     => 'multi-service/POST /api/v2/orders',
        groups    => $groups,
        correlate => \@correlate,
    };
}

sub _find_span_id {
    my ($groups, $service, $name) = @_;
    for my $g (@$groups) {
        next unless $g->{service} eq $service;
        for my $sp (@{ $g->{spans} }) {
            return $sp->{spanId} if $sp->{name} eq $name;
        }
    }
    return undef;
}

sub _trace_scenarios {
    return (
        #            service           root_name             status        children hours_ago dur_ms
        _simple_trace(service => 'api-gateway',     root_name => 'GET /users',          children => 3, hours_ago => 1,  dur_ms => 85),
        _simple_trace(service => 'api-gateway',     root_name => 'POST /orders',        children => 4, hours_ago => 3,  dur_ms => 210),
        _simple_trace(service => 'api-gateway',     root_name => 'GET /products',       status => STATUS_ERROR, children => 2, hours_ago => 6, dur_ms => 340),
        _simple_trace(service => 'billing-service', root_name => 'charge',              children => 1, hours_ago => 8,  dur_ms => 150),
        _simple_trace(service => 'billing-service', root_name => 'refund',              status => STATUS_ERROR, children => 0, hours_ago => 12, dur_ms => 45),
        _simple_trace(service => 'user-service',    root_name => 'authenticate',        children => 2, hours_ago => 16, dur_ms => 110),
        _simple_trace(service => 'user-service',    root_name => 'register',            children => 3, hours_ago => 20, dur_ms => 275),
        _simple_trace(service => 'catalog-service', root_name => 'search',              children => 5, hours_ago => 24, dur_ms => 190),
        _simple_trace(service => 'catalog-service', root_name => 'get-product-detail',  children => 1, hours_ago => 30, dur_ms => 60),
        _simple_trace(service => 'order-service',   root_name => 'create-order',        status => STATUS_ERROR, children => 4, hours_ago => 36, dur_ms => 420),
        _simple_trace(service => 'order-service',   root_name => 'list-orders',         children => 2, hours_ago => 40, dur_ms => 95),
        _simple_trace(service => 'notification',    root_name => 'send-email',          children => 0, hours_ago => 44, dur_ms => 30),
        _sample_loadgenerator_trace(),
        _deep_hierarchy_trace(),
        _orphan_spans_trace(),
        _orphan_subtree_trace(),
        _duplicate_span_id_trace(),
        _large_multiservice_trace(),
    );
}

sub run_traces {
    print "Sending traces to $endpoint/v1/traces ...\n";
    my @traces = _trace_scenarios();
    my @correlation;
    my $i = 0;
    for my $t (@traces) {
        $i++;
        my ($status, $err) = send_trace($endpoint, $t->{groups});
        my $spans = 0;
        $spans += scalar @{ $_->{spans} } for @{ $t->{groups} };
        if (defined $err) {
            printf "  [%2d] %-50s FAIL %s (%s)\n", $i, $t->{label}, $status, $err;
        } else {
            printf "  [%2d] %-50s %s  (spans: %d)\n", $i, $t->{label}, $status, $spans;
        }
        push @correlation, @{ $t->{correlate} // [] };
    }
    _write_correlation(\@correlation);
    print "Done. Sent ", scalar(@traces), " traces.\n";
}

# Log scenarios
# correlate => 1 prefers a same-service handoff and otherwise uses random IDs.

sub _log_scenarios {
    return (
        { svc => 'api-gateway',          text => 'TRACE', num => 1,  mins => 30,
          body => 'Entering middleware chain for request /api/v2/orders' },
        { svc => 'user-service',         text => 'DEBUG', num => 5,  mins => 28,
          body => 'Cache miss for user profile uid=a8f2c, falling back to DB' },
        { svc => 'catalog-service',      text => 'DEBUG', num => 7,  mins => 25,
          body => 'Resolved 42 product images from CDN in 38ms' },
        { svc => 'api-gateway',          text => 'INFO',  num => 9,  mins => 20, correlate => 1,
          body => 'Request completed: POST /api/v2/orders -> 201 Created (1.2s)' },
        { svc => 'order-service',        text => 'INFO',  num => 10, mins => 18,
          body => 'Order ORD-20260411-7829 created successfully with 3 line items' },
        { svc => 'notification-service', text => 'INFO',  num => 9,  mins => 15,
          body => 'Confirmation email queued for delivery to user@example.com' },
        { svc => 'shipping-service',     text => 'INFO',  num => 10, mins => 12,
          body => 'Pickup scheduled with carrier FedEx, tracking: 7948302841' },
        { svc => 'inventory-service',    text => 'WARN',  num => 13, mins => 10,
          body => 'Stock level below threshold: SKU-4412 has 3 remaining (threshold: 10)',
          attrs => [ attr('sku', 'SKU-4412'), attr('remaining', 3), attr('threshold', 10) ] },
        { svc => 'payment-service',      text => 'WARN',  num => 14, mins => 8,
          body => 'Payment gateway response time degraded: p99=2.3s (SLA: 1s)' },
        { svc => 'api-gateway',          text => 'WARN',  num => 13, mins => 5,
          body => 'Rate limit approaching for client app-id=mobile-ios: 890/1000 requests in window' },
        { svc => 'payment-service',      text => 'ERROR', num => 17, mins => 4, correlate => 1,
          body => 'Payment declined: card ending 4242, reason: insufficient_funds',
          attrs => [ attr('error.type', 'PaymentDeclinedError'), attr('card.last4', '4242') ] },
        { svc => 'order-service',        text => 'ERROR', num => 17, mins => 3,
          body => 'Failed to persist order: deadlock detected on orders table, retrying (attempt 2/3)' },
        { svc => 'user-service',         text => 'ERROR', num => 18, mins => 2,
          body => 'Authentication failed: JWT signature verification error for token issued by idp.example.com' },
        { svc => 'inventory-service',    text => 'FATAL', num => 21, mins => 1,
          body => 'Database connection pool exhausted: 0/50 connections available, all queries blocked' },
        { svc => 'api-gateway',          text => 'INFO',  num => 9,  mins => 22, correlate => 1,
          body => '{"method":"POST","path":"/api/v2/orders","status":201,"duration_ms":1247,"request_id":"req-f8a2b","user_id":"usr-9c4e1","items":3,"total_cents":15499}',
          attrs => [ attr('http.method', 'POST'), attr('http.route', '/api/v2/orders') ] },
        { svc => 'catalog-service',      text => 'INFO',  num => 10, mins => 14, event_name => 'price.recalculated',
          body => 'Product price recalculated after discount rules applied' },
        { svc => 'api-gateway',          text => 'INFO',  num => 9,  mins => 45,
          body => 'Health check passed: all upstream dependencies responding' },
        { svc => 'user-service',         text => 'INFO',  num => 9,  mins => 40,
          body => 'Session renewed for user usr-9c4e1, new expiry in 30m' },
        { svc => 'catalog-service',      text => 'WARN',  num => 13, mins => 35,
          body => 'Product description exceeds recommended length: SKU-8827 (4200 chars, limit 3000)' },
        { svc => 'order-service',        text => 'DEBUG', num => 6,  mins => 17,
          body => 'Computing order totals: subtotal=12499, tax=2340, shipping=660' },
        { svc => 'payment-service',      text => 'INFO',  num => 10, mins => 7,  correlate => 1,
          body => 'Refund processed: txn-88f2a, amount=5000 cents, reason=customer_request' },
        { svc => 'notification-service', text => 'ERROR', num => 17, mins => 6,
          body => 'SMTP connection timeout after 30s: smtp.mailer.example.com:587' },
        { svc => 'shipping-service',     text => 'DEBUG', num => 5,  mins => 11,
          body => 'Carrier rate query: FedEx=12.50, UPS=14.20, USPS=8.90 -- selected USPS' },
    );
}

# Prefer a correlation entry from the same service.
sub _pick_correlation {
    my ($correlation, $service) = @_;
    return undef unless @$correlation;
    my @same = grep { $_->{service} eq $service } @$correlation;
    my $pool = @same ? \@same : $correlation;
    return $pool->[ int(rand scalar @$pool) ];
}

sub run_logs {
    print "Sending logs to $endpoint/v1/logs ...\n";
    my $correlation = _read_correlation();
    printf "  (correlating against %d trace span(s) from %s)\n",
        scalar(@$correlation), $CORRELATION_FILE if @$correlation;

    my @specs = _log_scenarios();
    my $i = 0;
    for my $spec (@specs) {
        $i++;
        my $t_ns = $now_ns - $spec->{mins} * NS_PER_MIN;

        # Fall back to random IDs when no correlation handoff is available.
        my ($trace_id, $span_id);
        if ($spec->{correlate}) {
            if (my $hit = _pick_correlation($correlation, $spec->{svc})) {
                ($trace_id, $span_id) = ($hit->{trace_id}, $hit->{span_id});
            } else {
                ($trace_id, $span_id) = (Traces::trace_id(), Traces::span_id());
            }
        }

        my $record = log_record({
            t_ns            => $t_ns,
            severity_number => $spec->{num},
            severity_text   => $spec->{text},
            body            => $spec->{body},
            trace_id        => $trace_id,
            span_id         => $span_id,
            event_name      => $spec->{event_name},
            attributes      => $spec->{attrs},
        });

        my ($status, $err) = send_logs($endpoint, $spec->{svc}, [$record]);
        my $tag = $trace_id ? ' [trace]' : '';
        if (defined $err) {
            printf "  [%2d] %-22s %-6s FAIL %s (%s)\n", $i, $spec->{svc}, $spec->{text}, $status, $err;
        } else {
            printf "  [%2d] %-22s %-6s %s  (%d min ago)%s\n", $i, $spec->{svc}, $spec->{text}, $status, $spec->{mins}, $tag;
        }
    }
    print "Done. Sent ", scalar(@specs), " log records.\n";
}

run_metrics() if $do_metrics;
run_traces()  if $do_traces;
run_logs()    if $do_logs;
