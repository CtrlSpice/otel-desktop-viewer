-- DuckDB's decimal parser rounds once to binary64 (nearest, ties to even).
-- The direct HUGEINT -> DOUBLE cast does not. Decimal text preserves the exact
-- integer until that conversion, including -2^127, without a SQL bitwise fold.
create or replace macro hugeint_to_double(value_) as (
	value_::hugeint::varchar::double
)
