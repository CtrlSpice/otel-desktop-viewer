-- Exponential histogram negative region, emitted in CDF order (most negative first).
-- Source bucket at original position j covers [-base^(offset+j), -base^(offset+j-1));
-- list_reverse walks j from len down to 1 so output is numerically ascending.
--
-- Note: the OTLP wire format treats positives and negatives as independent
-- (not mirrored), but in practice the negative region is empty for the
-- common case (latency, byte counts, queue depth, ...). Only signed-value
-- instruments (temperature deltas, P&L, geo offsets) populate it. We handle
-- it correctly because the spec allows it and the formula is the same shape
-- as the positive region with sign-preserving math.
create or replace macro exp_neg_buckets(scale, offset_, counts) as (
		list_transform(list_reverse(counts), lambda c, i: {
			'lo': -pow(2.0, pow(2.0, -scale) * (offset_ + len(counts) - i + 1)),
			'hi': -pow(2.0, pow(2.0, -scale) * (offset_ + len(counts) - i)),
			'cnt': c
		})
	)
