-- exp_zero_cutoff: the highest bucket index wholly inside the zero region.
--
-- Bucket i at scale s covers (base^i, base^(i+1)], base = 2^(2^-s).
-- Its upper bound is inside zero threshold T when:
--
--	2^((i+1) * 2^-s) <= T
--	(i+1) * 2^-s     <= log2(T)
--	i                <= log2(T) * 2^s - 1
--
-- Therefore cutoff = floor(log2(T) * 2^s) - 1. NULL means no zero region.
create or replace macro exp_zero_cutoff(zero_threshold, scale) as (
    case
        when zero_threshold is null or zero_threshold <= 0 then null
        else cast(floor(log2(zero_threshold) * pow(2, scale)) as bigint) - 1
    end
)
