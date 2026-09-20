-- Decode the canonical D06 int64 string without passing through DOUBLE.
create or replace macro attribute_int64(value) as (
	case
		when json_extract_string(value, '$.kind') = 'int64'
			then try_cast(json_extract_string(value, '$.value') as bigint)
		else null::bigint
	end
)
