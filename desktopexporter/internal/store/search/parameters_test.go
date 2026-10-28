package search

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRebaseParametersPreservesSQLLiterals(t *testing.T) {
	const query = "select $1, $12, '$1''$2', \"$3\", $$literal $4$$, $tag$literal $5$tag$ /* $6 /* $7 */ */ -- $8\nwhere x = $1"
	const want = "select $4, $15, '$1''$2', \"$3\", $$literal $4$$, $tag$literal $5$tag$ /* $6 /* $7 */ */ -- $8\nwhere x = $4"
	require.Equal(t, want, RebaseParameters(query, 3))
	require.Equal(t, query, RebaseParameters(query, 0))
}
