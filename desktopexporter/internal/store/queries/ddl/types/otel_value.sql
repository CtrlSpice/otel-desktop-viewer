-- Shared native representation for OTel AnyValue. Construction is centralized in
-- otel_value_from_nodes; VARIANT retains each scalar's OTel type recursively.
create type otel_value as variant
