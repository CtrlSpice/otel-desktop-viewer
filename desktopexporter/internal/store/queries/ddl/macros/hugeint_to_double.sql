-- Correctly rounded signed HUGEINT -> binary64 (nearest, ties to even), as in
-- ECMAScript Number(BigInt). DuckDB's native HUGEINT cast is not correctly rounded.
-- Keep the magnitude unsigned and exact: -(n + 1) + 1 also handles -2^127.
-- The length of its binary representation selects the shift without a floating
-- logarithm. Retain 53 leading bits, then use the exact remainder to round once.
-- Only the rounded significand (at most 2^53) crosses an integer -> DOUBLE cast;
-- scaling by a power of two and applying the sign are exact throughout this domain.
-- Singleton list lambdas bind intermediate values without correlated subqueries.
create or replace macro hugeint_to_double(value_) as (
	list_transform([value_::hugeint], lambda n:
		list_transform([case when n < 0 then (-(n + 1))::uhugeint + 1
			else n::uhugeint end], lambda magnitude:
			list_transform([greatest(0, len(bin(magnitude)) - 53)], lambda shift_:
				list_transform([1::uhugeint << shift_::uhugeint], lambda unit_:
					list_transform([struct_pack(
						q := magnitude // unit_, r := magnitude % unit_
					)], lambda parts:
						(parts.q + case when shift_ > 0 and
							(parts.r > unit_ // 2 or
							 (parts.r = unit_ // 2 and parts.q % 2 = 1))
							then 1 else 0 end)::ubigint::double
						* pow(2.0, shift_) * case when n < 0 then -1.0 else 1.0 end
					)[1]
				)[1]
			)[1]
		)[1]
	)[1]
)
