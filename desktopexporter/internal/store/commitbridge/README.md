# DuckDB commit bridge

Each store owns one DuckDB database and its connections. This static extension
registers `ClientContextState::TransactionCommit` on each connection.
Successful transactions that modified a database advance
the store's in-memory revision and wake its subscribers. The server-sent events
(SSE) handler sends
the revision to browsers, which can rerun their queries. Reads and rollbacks
leave the revision unchanged. Queries and SSE writes run outside the native
callback.

The C++ headers in `include/` come from DuckDB source
`d54cbbe3df17e8e5ce68312f483b4155836112aa`, the source recorded by
`go.duckdb.org/libduckdb` v0.20000.0. The extension checks the engine's source ID
before accessing C++ layouts; a mismatch fails store initialization. Keep the
headers and static library on the same source revision when upgrading DuckDB.
Its MIT licence is in `LICENSE.duckdb`.

To reproduce the headers, run these commands from the repository root:

```sh
curl -L https://github.com/duckdb/duckdb/archive/d54cbbe3df17e8e5ce68312f483b4155836112aa.tar.gz -o duckdb-source.tar.gz
go run ./desktopexporter/internal/store/commitbridge/generate duckdb-source.tar.gz
go test ./desktopexporter/internal/store/commitbridge
```

The Go command verifies the archive checksum and copies the bridge's transitive
header dependencies without modifying them.
