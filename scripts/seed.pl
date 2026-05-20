#!/usr/bin/env perl

# ============================================================================
# seed.pl -- populate the OTel Desktop Viewer with synthetic telemetry.
#
# Usage:
#   ./scripts/seed.pl --metrics                          # seed metrics
#   ./scripts/seed.pl --metrics --endpoint http://...    # custom endpoint
#   ./scripts/seed.pl --metrics --seed 7                 # deterministic
#
# Currently implemented signals: --metrics
# Pending: --traces, --logs, --all (port from existing shell scripts)
#
# Realism: every metric is built from composable shape functions
# (see lib/Shapes.pm). Default scenarios cover diurnal load, slow
# creep, sudden incidents, sawtooth ramps over >24h, and a sampling
# of metrics with aggregationTemporality=Unspecified to exercise the
# fun-error path.
# ============================================================================

use strict;
use warnings;
use FindBin qw($Bin);
use lib "$Bin/lib";

use Getopt::Long;

use OTLP    qw(attr now_ns);
use Shapes  qw(:all);
use Metrics qw(:all);

# ----------------------------------------------------------------------------
# Options
# ----------------------------------------------------------------------------

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

# now_s is the script's "wall clock zero". Every shape's t=0 maps to
# (now_s - duration), so the most recent datapoint lands at now_s.
my $now_s = time;

# ----------------------------------------------------------------------------
# Metric scenarios
#
# Each scenario is a small lambda that returns ($service, $metric).
# Keeping them as functions (not data) means each scenario can pick
# its own shape composition, RNG draw, and attribute fan-out without
# trying to cram everything into a config schema.
# ----------------------------------------------------------------------------

# Helper: shift a sample's t_s (which is "seconds from t=0 of the
# scenario") to absolute Unix epoch seconds. Shapes don't know about
# wall clock; metrics do.
sub _absolute {
    my ($end_s, $duration_s, @points) = @_;
    my $start = $end_s - $duration_s;
    return map { { t_s => $start + $_->{t_s}, value => $_->{value} } } @points;
}

sub _scenarios {
    my ($rng) = @_;

    return (
        # ----- Gauge: classic CPU diurnal across last 4h, 30s step -----
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

        # ----- Gauge: memory creep + small noise, last 6h -----
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

        # ----- Gauge: queue depth with two clear incidents in last 2h -----
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

        # ----- Sum (cumulative monotonic): request count, 30min, fan-out
        #       across method × route × status_class (30 streams). Each
        #       stream gets its own diurnal-modulated rate; the higher-
        #       traffic routes have larger baselines, and one stream gets
        #       a noticeable bump so a couple of legend rows stand out
        #       from the baseline pack. Cumulative => each stream runs
        #       its own accumulator independently. -----
        sub {
            my $duration = 30 * 60;
            my $step     = 60;
            # (method, baseline_rps, share_of_5xx, route_weight)
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
                        # Effective per-second rate for this stream.
                        my $share = $s->{name} eq '5xx' ? $m->{err_pct} : (1 - $m->{err_pct});
                        my $rps   = $m->{base} * $r->{weight} * $share;

                        # Per-stream shape: own baseline + diurnal phase
                        # offset (route-derived) + independent noise draw.
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

                        # Cumulative accumulator. Each stream starts at a
                        # different running total so the chart isn't all
                        # bunched at the same y-intercept.
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

            return ('api-gateway', sum_metric_streams({
                name        => 'http.server.request.count',
                unit        => '{requests}',
                description => 'Total HTTP requests received, by method/route/status',
                step_s      => $step,
                temporality => AGG_CUMULATIVE,
                monotonic   => 1,
            }, \@streams));
        },

        # ----- Sum (delta): error count per minute, last hour, fan-out
        #       across service × error_class (12 streams). Most streams
        #       are quiet noise; two get clear incident spikes so the
        #       chart has both a busy "everything's fine" baseline and
        #       a couple of obvious offenders. Delta semantics: each
        #       point is the count *in* that minute, not running total. -----
        sub {
            my $duration = 60 * 60;
            my $step     = 60;
            my @services = qw(api-gateway payment-service notification-service shipping-service);
            my @classes  = qw(5xx timeout dependency);   # error class

            # Pick two (service, class) pairs to spike. Indices into the
            # 4 × 3 = 12 grid; the rest get plain noisy baselines.
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
                    # Per-stream baseline: 5xx is loudest, timeout
                    # moderate, dependency rare. Scale by service so the
                    # payment-service is twitchier than shipping.
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

            return ('api-gateway', sum_metric_streams({
                name        => 'http.server.error.count',
                unit        => '{errors}',
                description => 'Server-side errors per interval, by service/class',
                step_s      => $step,
                temporality => AGG_DELTA,
                monotonic   => 1,
            }, \@streams));
        },

        # ----- Sum: UNSPECIFIED temporality -- exercises the fun error.
        # Rising counter (creep) on purpose: with Unspecified, a viewer
        # cannot decide whether the values are running totals or
        # per-interval counts. Reading [14000, 14060, 14120, ...] as
        # Cumulative -> ~60 events/min; as Delta -> ~14000 events/min.
        # Two-orders-of-magnitude gap. Make the ambiguity visible.
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

        # ----- Histogram (delta): HTTP request duration, 90 minutes,
        #       1-min step, 3 routes (3 streams). Mid-window latency spike
        #       on /orders. -----
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
            # Combine all streams into one metric (one dataPoints array
            # with attribute-distinguished dps per timestamp).
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
            # Build datapoints by hand because each one carries different attrs.
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

        # ----- Histogram: UNSPECIFIED temporality (single stream, short) -----
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

        # ----- Sawtooth gauge over 48h to exercise date-aware axis labels -----
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

        # ----- Exponential histogram (delta) -----
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

        # ----- Exponential histogram: UNSPECIFIED -----
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

# ----------------------------------------------------------------------------
# Drivers (one per signal kind)
# ----------------------------------------------------------------------------

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
        # Find which kind key is present for the printed line.
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

sub run_traces {
    print "traces: not yet ported to seed.pl -- run scripts/seed-traces.sh for now.\n";
}

sub run_logs {
    print "logs: not yet ported to seed.pl -- run scripts/seed-logs.sh for now.\n";
}

# ----------------------------------------------------------------------------
# Dispatch
# ----------------------------------------------------------------------------

run_metrics() if $do_metrics;
run_traces()  if $do_traces;
run_logs()    if $do_logs;
