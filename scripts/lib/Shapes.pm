package Shapes;

# Composable generators for synthetic time-series values.

use strict;
use warnings;

use Exporter qw(import);
use Math::Trig qw(pi);
use List::Util qw(sum0);

use ScreamingSnake;

use constant TAU => 2 * pi;

our @EXPORT_OK = qw(
    make_rng
    diurnal
    sawtooth
    incident
    creep
    constant
    noisy
    clamp
    compose
    sample
    TAU
);
our %EXPORT_TAGS = ( all => \@EXPORT_OK );

# Closure-scoped 32-bit LCG; 64-bit multiplication would promote to double.
sub make_rng {
    my ($seed) = @_;
    $seed //= 42;
    my $state = $seed & 0xFFFFFFFF;
    return sub {
        my ($n) = @_;
        $state = ($state * 1664525 + 1013904223) & 0xFFFFFFFF;
        my $f = $state / 4294967296.0;
        return defined $n ? $f * $n : $f;
    };
}

# Sinusoid shifted right by phase_s seconds.
sub diurnal {
    my ($p) = @_;
    my $amp     = $p->{amplitude} // 1;
    my $base    = $p->{baseline}  // 0;
    my $period  = $p->{period_s}  // 86400;
    my $phase   = $p->{phase_s}   // 0;
    return sub {
        my ($t) = @_;
        return $base + $amp * sin(TAU * ($t - $phase) / $period);
    };
}

# Linear ramp from 0 to amplitude, resetting every period_s.
sub sawtooth {
    my ($p) = @_;
    my $amp    = $p->{amplitude} // 1;
    my $base   = $p->{baseline}  // 0;
    my $period = $p->{period_s}  // 3600;
    return sub {
        my ($t) = @_;
        my $frac = ($t % $period) / $period;
        return $base + $amp * $frac;
    };
}

# Ramp from baseline to peak, hold, then recover to baseline.
sub incident {
    my ($p) = @_;
    my $base     = $p->{baseline}    // 0;
    my $peak     = $p->{peak}        // 1;
    my $start    = $p->{start_s}     // 0;
    my $ramp     = $p->{ramp_s}      // 60;
    my $hold     = $p->{hold_s}      // 300;
    my $recovery = $p->{recovery_s}  // 600;
    my $hold_end = $start + $ramp + $hold;
    my $end      = $hold_end + $recovery;
    return sub {
        my ($t) = @_;
        if ($t < $start || $t >= $end) {
            return $base;
        } elsif ($t < $start + $ramp) {
            my $f = ($t - $start) / $ramp;
            return $base + ($peak - $base) * $f;
        } elsif ($t < $hold_end) {
            return $peak;
        } else {
            my $f = ($t - $hold_end) / $recovery;
            return $base + ($peak - $base) * (1 - $f);
        }
    };
}

# Linear drift.
sub creep {
    my ($p) = @_;
    my $base  = $p->{baseline}     // 0;
    my $slope = $p->{slope_per_s}  // 0;
    return sub {
        my ($t) = @_;
        return $base + $slope * $t;
    };
}

sub constant {
    my ($v) = @_;
    return sub { return $v };
}

# Multiplicative noise from the mean of four uniform draws.
sub noisy {
    my ($shape, $fraction, $rng) = @_;
    $fraction //= 0.05;
    return sub {
        my ($t) = @_;
        my $v = $shape->($t);
        my $u = ($rng->() + $rng->() + $rng->() + $rng->()) / 4 - 0.5;  # ~ N(0, 1/12)
        return $v * (1 + 2 * $fraction * $u);
    };
}

sub clamp {
    my ($shape, $min, $max) = @_;
    return sub {
        my ($t) = @_;
        my $v = $shape->($t);
        $v = $min if defined $min && $v < $min;
        $v = $max if defined $max && $v > $max;
        return $v;
    };
}

# Sum shapes pointwise.
sub compose {
    my @shapes = @_;
    return sub {
        my ($t) = @_;
        return sum0 map { $_->($t) } @shapes;
    };
}

# Sample the inclusive interval and return a list of {t_s, value} records.
sub sample {
    my ($shape, $start_s, $end_s, $step_s) = @_;
    my @out;
    for (my $t = $start_s; $t <= $end_s; $t += $step_s) {
        push @out, { t_s => $t, value => $shape->($t) };
    }
    return @out;
}

THE_END();
