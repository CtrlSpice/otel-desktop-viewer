-- Encode native DOUBLE directly; ProtoJSON names the three non-finite values.
create or replace macro otlp_double_json(value) as (
	case
		when isnan(value) then to_json('NaN')
		when value = 'Infinity'::double then to_json('Infinity')
		when value = '-Infinity'::double then to_json('-Infinity')
		else to_json(value)
	end
)
