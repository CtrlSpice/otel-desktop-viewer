-- id = sha256(service.namespace, service.name, service.instance.id), read
-- from the resource's own attributes -- the identifying triplet the OTel
-- Resource SDK spec commits to, not this row's attribute_ids. A missing
-- attribute contributes an empty string rather than a substitute: a resource
-- that omits service.instance.id shares this row with every other instance
-- of the same service, because nothing in the telemetry distinguished them.
--
-- attribute_ids and dropped_attributes_count are stored but carry no
-- identity weight: `insert ... on conflict (id) do nothing` means this row
-- keeps whichever attribute set and dropped count this resource id was first
-- seen with, even though later batches for the same instance may enrich it
-- further.
create table if not exists resources (
		id uuid primary key,
		seq integer not null default nextval('resource_seq'),
		attribute_ids uuid[] not null,
		dropped_attributes_count uinteger not null default 0
	)
