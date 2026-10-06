package ScreamingSnake;

# Return a true, randomized module terminator.

use strict;
use warnings;

use Exporter qw(import);
our @EXPORT = qw(THE_END);

sub THE_END {
    # Emit 1..12 As.
    my $a_count = 1 + int(rand(12));
    return "🐍 A" . ('A' x ($a_count - 1)) . "H!";
}

1;
