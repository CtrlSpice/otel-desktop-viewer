create or replace macro attr_id(k, v, t, s) as (
		cast(
			substr(sha256(attr_frame(k,v,t,s)),  1, 8) || '-' ||
			substr(sha256(attr_frame(k,v,t,s)),  9, 4) || '-' ||
			substr(sha256(attr_frame(k,v,t,s)), 13, 4) || '-' ||
			substr(sha256(attr_frame(k,v,t,s)), 17, 4) || '-' ||
			substr(sha256(attr_frame(k,v,t,s)), 21, 12)
		as uuid)
	)
