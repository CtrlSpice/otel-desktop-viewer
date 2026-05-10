package Shapes;

# ============================================================================
# Shapes.pm -- generator functions for synthetic time-series values.
#
# Why this module exists:
#   Realistic-looking telemetry needs realistic-looking shapes:
#   diurnal traffic, slow upward creep, sudden incidents, noise on
#   top of everything. Rather than hand-coding each metric's data,
#   we build it from a small library of shape functions that compose.
#
# What you get (FP-shaped: every constructor returns a function):
#   - make_rng($seed)                      -> \&rand_fn        (deterministic [0,1))
#   - diurnal({ amplitude, baseline, period_s, phase_s })
#                                          -> \&shape_fn       (t_s -> value)
#   - sawtooth({ amplitude, baseline, period_s })
#                                          -> \&shape_fn
#   - incident({ baseline, peak, start_s, ramp_s, hold_s, recovery_s })
#                                          -> \&shape_fn
#   - creep({ baseline, slope_per_s })     -> \&shape_fn
#   - constant($value)                     -> \&shape_fn
#   - noisy($shape, $fraction, $rng)       -> \&shape_fn       (wraps a shape)
#   - clamp($shape, $min, $max)            -> \&shape_fn
#   - compose(@shapes)                     -> \&shape_fn       (sum)
#   - sample($shape, $start_s, $end_s, $step_s)
#                                          -> \@points         ({ t_s, value })
#
# The whole point of this module: complex behaviour by composing
# small functions. e.g. a noisy diurnal load with a slow creep is
#
#     compose(diurnal({...}), creep({...}))
#       |> noisy(0.05, $rng)
#
# (Perl 5 doesn't have a pipe operator; you write that as
#  noisy(compose(diurnal({...}), creep({...})), 0.05, $rng) )
# ============================================================================

use strict;
use warnings;

use Exporter qw(import);
use Math::Trig qw(pi);
use List::Util qw(sum0);

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

# ----------------------------------------------------------------------------
# Deterministic RNG
# ----------------------------------------------------------------------------

# A 32-bit linear congruential generator (Numerical Recipes
# constants). Tiny, fast, and crucially has its own state -- so we
# can reproduce the exact same data across runs by passing the same
# seed. Perl's built-in srand/rand uses a global state, which would
# conflict with anything else in the process; a closure-scoped LCG
# keeps our determinism local.
#
# Why 32 bits and not 64: Perl's integer arithmetic silently promotes
# to floating point on overflow, so a 64-bit LCG saturates to
# all-ones after one multiply. Staying inside 32 bits sidesteps that
# entirely. Quality is fine for fake telemetry; it'd be wrong for
# anything cryptographic.
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

# ----------------------------------------------------------------------------
# Atomic shapes (constructors that return a t -> value function)
# ----------------------------------------------------------------------------

# Sinusoidal. Peaks once per period_s, troughs half a period later.
# Phase shifts the curve right by phase_s seconds.
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

# Linear ramp from 0 to amplitude over period_s, then resets.
# Useful for queue-depth-style metrics or for >24h axis testing where
# you want an obvious visual repeat-every-N-hours pattern.
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

# An incident: quiet baseline, sharp ramp up to peak over ramp_s,
# hold at peak for hold_s, decay back over recovery_s. start_s is
# where the ramp begins (in the t coordinate the shape will be called
# with).
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

# Slow linear drift. Use to model "memory grows 100 KB/s" or the
# slow background trend underneath a diurnal pattern.
sub creep {
    my ($p) = @_;
    my $base  = $p->{baseline}     // 0;
    my $slope = $p->{slope_per_s}  // 0;
    return sub {
        my ($t) = @_;
        return $base + $slope * $t;
    };
}

# Constant value. Boring on its own; useful as a baseline you
# compose noise onto.
sub constant {
    my ($v) = @_;
    return sub { return $v };
}

# ----------------------------------------------------------------------------
# Higher-order shapes (functions that take and return shapes)
# ----------------------------------------------------------------------------

# Wrap a shape with multiplicative noise. The fraction is the rough
# +/- swing as a proportion of the underlying value (0.05 = +/- 5%).
# Approximates a normal distribution by averaging four uniforms
# (cheap CLT) -- not statistically rigorous, just less obviously
# uniform than a single rand() would look.
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

# Clamp a shape's output to [min, max]. Useful after composing
# several shapes that could in principle produce a negative request
# count.
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

# Sum N shapes pointwise. The bread-and-butter composition: any
# realistic-looking metric is "background + diurnal + occasional
# incident + noise", and that's just compose(constant, diurnal,
# incident) wrapped in noisy().
#
# FP detail: this returns a closure that captures @shapes by ref via
# the lexical -- so the returned function keeps working even after
# the caller's @shapes goes out of scope.
sub compose {
    my @shapes = @_;
    return sub {
        my ($t) = @_;
        return sum0 map { $_->($t) } @shapes;
    };
}

# ----------------------------------------------------------------------------
# Sampling
# ----------------------------------------------------------------------------

# Walk a shape across [start_s, end_s] at step_s intervals, producing
# a list of { t_s => ..., value => ... } records. This is the only
# place that turns a shape function back into concrete data; metric
# constructors consume the result.
#
# Returns a list (not a listref) so callers can `map` over it
# directly. Caller pays a copy if they want to store it -- fine for
# our scale (a few thousand points max).
sub sample {
    my ($shape, $start_s, $end_s, $step_s) = @_;
    my @out;
    for (my $t = $start_s; $t <= $end_s; $t += $step_s) {
        push @out, { t_s => $t, value => $shape->($t) };
    }
    return @out;
}

'This is the way.';
