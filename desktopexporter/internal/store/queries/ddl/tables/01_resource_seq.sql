-- seq is the short, store-stable key the wire format uses instead of a
-- 36-char uuid. Sequence values are never reused, so a client cache can
-- miss but never be wrong.
create sequence if not exists resource_seq
