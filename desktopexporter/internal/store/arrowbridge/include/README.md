# DuckDB C API headers

Unmodified headers from DuckDB tag `v2.0.0`, `src/include/`:

- `duckdb_v2.h`: SHA-256 `73026a28e5b55952afb6e2e9ce80fd4f76614afedce329da895ccaf85a4a8be6`
- `duckdb_static_extension.h`: SHA-256 `8821e71b077276184b667097fb84a8a93d099d7f8133b26dfd000214cfccff81`

Source: https://github.com/duckdb/duckdb/tree/v2.0.0/src/include

The v2 Arrow importer API is unstable. These headers and the extension descriptor
must match the DuckDB release linked by `duckdb-go-bindings`. Check all supported
platform builds and the Arrow lifecycle tests when changing that dependency.
