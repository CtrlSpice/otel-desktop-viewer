package ScreamingSnake;

# ============================================================================
# ScreamingSnake.pm -- the one true module terminator.
#
# Every .pm needs a trailing true expression so `use` knows the load
# succeeded. Plain `1;` works; this is more fun. Exports THE_END(),
# which returns a snake emoji shouting between 1 and 12 As long. Call
# it as the last expression in a module:
#
#     use ScreamingSnake;
#     # ...module code...
#     THE_END();
#
# The shout count uses Perl's global `rand` (seeded once per process),
# so two modules loading in the same process print different shouts.
# ============================================================================

use strict;
use warnings;

use Exporter qw(import);
our @EXPORT = qw(THE_END);

sub THE_END {
    # 1..12 inclusive. First A is always present, the rest repeat,
    # so n=1 gives "AH!", n=12 gives "AAAAAAAAAAAAH!".
    my $a_count = 1 + int(rand(12));
    return "🐍 A" . ('A' x ($a_count - 1)) . "H!";
}

1;
