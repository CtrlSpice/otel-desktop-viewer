create or replace macro attr_id(k, v) as (
		cast(
			substr(sha256(attr_frame(k,v)),  1, 8) || '-' ||
			substr(sha256(attr_frame(k,v)),  9, 4) || '-' ||
			substr(sha256(attr_frame(k,v)), 13, 4) || '-' ||
			substr(sha256(attr_frame(k,v)), 17, 4) || '-' ||
			substr(sha256(attr_frame(k,v)), 21, 12)
		as uuid)
	)
