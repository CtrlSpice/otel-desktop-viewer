-- One row per distinct typed (key, value). id is SHA-256 over
-- length-prefixed fields, truncated to 16 bytes.
create table if not exists attributes (
		id uuid primary key,
		key varchar not null,
		-- VARIANT null represents a received OTel empty value.
		value otel_value
	)
