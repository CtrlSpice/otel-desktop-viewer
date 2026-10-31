package ingest

import (
	"bytes"
	"testing"

	pv "github.com/apache/arrow-go/v18/parquet/variant"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestVariantBytesOwnBuilderAndPdataStorage(t *testing.T) {
	v := pcommon.NewValueMap()
	v.Map().PutEmptyBytes("bytes").FromRaw([]byte{0, 255, 128})
	v.Map().PutEmptySlice("array").AppendEmpty().SetInt(9007199254740993)
	var b pv.Builder
	require.NoError(t, appendValue(&b, v))
	first, err := finishValue(&b)
	require.NoError(t, err)
	want := bytes.Clone(first)
	v.Map().Clear()
	b.Reset()
	require.NoError(t, b.AppendString("a different value overwrites reused builder storage"))
	_, err = finishValue(&b)
	require.NoError(t, err)
	require.Equal(t, want, first)
}
