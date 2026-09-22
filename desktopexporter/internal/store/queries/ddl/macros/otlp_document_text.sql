-- A serializer must not emit JSON null for an absent protobuf field or inside
-- a repeated field. Empty AnyValue is {}, so rejecting JSON null does not reject
-- a valid empty value.
create or replace macro otlp_document_text(document) as (
	case
		when document is null then null
		when json_contains(document, 'null'::json)
			then error('OTLP document contains SQL NULL')
		else document::varchar
	end
)
