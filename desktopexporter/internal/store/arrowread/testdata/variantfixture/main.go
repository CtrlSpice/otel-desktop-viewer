// variantfixture writes a fresh native Arrow stream for the browser compatibility test.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowbridge"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowread"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/duckdb/duckdb-go/v2"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

type example struct {
	name  string
	value pcommon.Value
}

func examples() []example {
	values := []example{
		{"empty", pcommon.NewValueEmpty()},
		{"true", pcommon.NewValueBool(true)},
		{"false", pcommon.NewValueBool(false)},
		{"text", pcommon.NewValueStr("\ufeff雪\x00literal")},
		{"empty-string", pcommon.NewValueStr("")},
		{"double-one", pcommon.NewValueDouble(1)},
		{"negative-zero", pcommon.NewValueDouble(math.Copysign(0, -1))},
		{"nan", pcommon.NewValueDouble(math.Float64frombits(0x7ff8000000000042))},
		{"positive-infinity", pcommon.NewValueDouble(math.Inf(1))},
		{"negative-infinity", pcommon.NewValueDouble(math.Inf(-1))},
		{"smallest-double", pcommon.NewValueDouble(math.SmallestNonzeroFloat64)},
		{"largest-double", pcommon.NewValueDouble(math.MaxFloat64)},
		{"empty-map", pcommon.NewValueMap()},
		{"empty-array", pcommon.NewValueSlice()},
	}
	for _, n := range []int64{0, 1, -1, -128, 127, -129, 128, -32768, 32767, -32769, 32768, math.MinInt32, math.MaxInt32, 9007199254740993, math.MinInt64, math.MaxInt64} {
		values = append(values, example{fmt.Sprintf("integer-%d", n), pcommon.NewValueInt(n)})
	}
	for _, data := range [][]byte{nil, {0, 255, 128}} {
		value := pcommon.NewValueEmpty()
		value.SetEmptyBytes().FromRaw(data)
		values = append(values, example{fmt.Sprintf("bytes-%d", len(data)), value})
	}
	object := pcommon.NewValueMap()
	object.Map().PutInt("__proto__", 9007199254740993)
	object.Map().PutDouble("constructor", 1)
	object.Map().PutStr("", "empty key")
	object.Map().PutStr("a\x00b", "NUL key")
	object.Map().PutStr("\ue000", "BMP")
	object.Map().PutStr("\U00010000", "non-BMP")
	items := object.Map().PutEmptySlice("items")
	items.AppendEmpty().SetInt(1)
	items.AppendEmpty().SetDouble(1)
	items.AppendEmpty().SetDouble(math.Copysign(0, -1))
	items.AppendEmpty().SetDouble(math.Float64frombits(0x7ff8000000000042))
	items.AppendEmpty().SetEmptyBytes().FromRaw([]byte{0, 255})
	items.AppendEmpty().SetEmptyMap()
	items.AppendEmpty().SetEmptySlice()
	items.AppendEmpty()
	values = append(values, example{"object", object})
	wide := pcommon.NewValueMap()
	for i := range 300 {
		wide.Map().PutInt(fmt.Sprintf("key-%03d", i), int64(i))
	}
	values = append(values, example{"wide-map", wide})
	for _, nesting := range []string{"arrays", "maps", "mixed"} {
		value := pcommon.NewValueEmpty()
		child := value
		for level := range 200 {
			if nesting == "maps" || nesting == "mixed" && level%2 == 0 {
				fields := child.SetEmptyMap()
				fields.PutBool("flag", false)
				child = fields.PutEmpty("child")
			} else {
				items := child.SetEmptySlice()
				items.AppendEmpty().SetBool(false)
				child = items.AppendEmpty()
			}
		}
		child.SetInt(9007199254740993)
		values = append(values, example{"deep-" + nesting, value})
	}
	return values
}

func write(ctx context.Context, path string) error {
	connector, err := duckdb.NewConnector("", nil)
	if err != nil {
		return err
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	conn, err := connector.Connect(ctx)
	if err != nil {
		return err
	}
	if err := errors.Join(arrowbridge.Load(ctx, conn), conn.Close()); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `create table fixture(id bigint, name varchar, payload variant)`); err != nil {
		return err
	}
	values := examples()
	for id, value := range values {
		encoded, err := ingest.ValueBytes(value.value)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `insert into fixture values ($1, $2, variant_bytes_to_variant($3::blob))`, id, value.name, encoded); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, `insert into fixture values ($1, 'sql-null', null)`, len(values)); err != nil {
		return err
	}
	result, err := arrowread.Read(ctx, db, `with all_values as (
		select * from fixture
		union all select 1000+i, 'batch-' || i::varchar, i::bigint::variant from range(9001) t(i)
	)
	select id, name, payload as value, [payload, null::variant] as samples,
		[{'timestamp': 18446744073709551615::ubigint, 'payload': payload}] as nested
	from all_values order by id`)
	if err != nil {
		return err
	}
	defer result.Close()
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	return errors.Join(result.Write(ctx, file), file.Close())
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: variantfixture output.arrow")
		os.Exit(2)
	}
	if err := write(context.Background(), os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
