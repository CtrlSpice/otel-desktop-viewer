<script lang="ts">
  import type { DataPoint } from '@/types/api-types'
  import DetailProperty from '@/components/shared/DetailProperty.svelte'
  import { formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'

  let { datapoint: dp }: { datapoint: DataPoint } = $props()
  const time = getTimeContext()
  const doubleText = (value: number) =>
    Object.is(value, -0) ? '-0' : String(value)
</script>

<table class="detail-fields w-full" aria-label="Datapoint fields">
  <tbody>
    <DetailProperty
      fieldName="timeUnixNano"
      fieldType="fixed64"
      fieldValue={dp.timestamp.toString()}
      timestampLabel={formatTimestamp(dp.timestamp, time.tz, 'milliseconds')}
    />
    <DetailProperty
      fieldName="startTimeUnixNano"
      fieldType="fixed64"
      fieldValue={dp.startTime.toString()}
      timestampLabel={formatTimestamp(dp.startTime, time.tz, 'milliseconds')}
    />
    {#if dp.metricType === 'Gauge' || dp.metricType === 'Sum'}
      {#if dp.intValue !== null}<DetailProperty
          fieldName="asInt"
          fieldType="int64"
          fieldValue={dp.intValue.toString()}
        />{/if}
      {#if dp.doubleValue !== null}<DetailProperty
          fieldName="asDouble"
          fieldType="double"
          fieldValue={doubleText(dp.doubleValue)}
        />{/if}
    {:else}
      <DetailProperty
        fieldName="count"
        fieldType="uint64"
        fieldValue={dp.count.toString()}
      />
      {#each ['sum', 'min', 'max'] as field}
        {@const value = dp[field as 'sum' | 'min' | 'max']}
        {#if value !== null}<DetailProperty
            fieldName={field}
            fieldType="double"
            fieldValue={doubleText(value)}
          />{/if}
      {/each}
      {#if dp.metricType === 'Histogram'}
        <DetailProperty
          fieldName="explicitBounds"
          fieldType="double[]"
          fieldValue={`[${dp.explicitBounds.map(doubleText).join(',')}]`}
          jsonText={`[${dp.explicitBounds.map(doubleText).join(',')}]`}
        />
        <DetailProperty
          fieldName="bucketCounts"
          fieldType="uint64[]"
          fieldValue={`[${dp.bucketCounts.join(',')}]`}
          jsonText={`[${dp.bucketCounts.join(',')}]`}
        />
      {:else}
        <DetailProperty
          fieldName="scale"
          fieldType="int32"
          fieldValue={String(dp.scale)}
        />
        <DetailProperty
          fieldName="zeroCount"
          fieldType="uint64"
          fieldValue={dp.zeroCount.toString()}
        />
        <DetailProperty
          fieldName="zeroThreshold"
          fieldType="double"
          fieldValue={doubleText(dp.zeroThreshold)}
        />
        <DetailProperty
          fieldName="positive.offset"
          fieldType="int32"
          fieldValue={String(dp.positiveBucketOffset)}
        />
        <DetailProperty
          fieldName="positive.bucketCounts"
          fieldType="uint64[]"
          fieldValue={`[${dp.positiveBucketCounts.join(',')}]`}
          jsonText={`[${dp.positiveBucketCounts.join(',')}]`}
        />
        <DetailProperty
          fieldName="negative.offset"
          fieldType="int32"
          fieldValue={String(dp.negativeBucketOffset)}
        />
        <DetailProperty
          fieldName="negative.bucketCounts"
          fieldType="uint64[]"
          fieldValue={`[${dp.negativeBucketCounts.join(',')}]`}
          jsonText={`[${dp.negativeBucketCounts.join(',')}]`}
        />
      {/if}
    {/if}
    <DetailProperty
      fieldName="flags"
      fieldType="uint32"
      fieldValue={String(dp.flags)}
    />
  </tbody>
</table>
