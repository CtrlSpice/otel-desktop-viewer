-- Keep ordinary doubles as compact JSON numbers. Hexadecimal IEEE-754 bits
-- preserve negative zero and non-finite values that JSON cannot represent.
create or replace macro double_wire_json(value) as (
	case
		when value is null then null::json
		when isfinite(value) and not (value = 0 and signbit(value))
			then to_json(value)
		else to_json('0x' || lower(hex((value::double)::bit::blob)))
	end
)
