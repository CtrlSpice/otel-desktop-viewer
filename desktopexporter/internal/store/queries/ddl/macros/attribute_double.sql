-- Decode the canonical D06 double payload. Ordinary values are JSON numbers;
-- negative zero and non-finite values are hexadecimal IEEE-754 bits.
create or replace macro attribute_double(value) as (
	case
		when json_extract_string(value, '$.kind') = 'double' then
			case
				when starts_with(json_extract_string(value, '$.value'), '0x') then
					try((unhex(substr(json_extract_string(value, '$.value'), 3))::bit)::double)
				else try_cast(json_extract_string(value, '$.value') as double)
			end
		else null::double
	end
)
