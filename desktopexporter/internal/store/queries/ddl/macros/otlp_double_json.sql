-- Convert a stored canonical double to its standard OTLP JSON representation.
-- Finite JSON numbers remain numbers; hexadecimal bits recover negative zero
-- and the three non-finite strings required by ProtoJSON.
create or replace macro otlp_double_json(encoded) as (
	case
		when json_type(json_extract(encoded, '$.value')) <> 'VARCHAR'
			then json_extract(encoded, '$.value')
		when isnan(attribute_double(encoded)) then to_json('NaN')
		when attribute_double(encoded) = 'Infinity'::double then to_json('Infinity')
		when attribute_double(encoded) = '-Infinity'::double then to_json('-Infinity')
		else to_json(attribute_double(encoded))
	end
)
