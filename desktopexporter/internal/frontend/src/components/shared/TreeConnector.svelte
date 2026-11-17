<script lang="ts">
  let {
    kind,
    start = '50%',
    reach = 'var(--waterfall-gutter-col)',
    targetSize = 'var(--waterfall-gutter-hub)',
    broken = false,
  }: {
    kind: 'stem' | 'passthrough' | 'tee' | 'elbow' | 'arm' | 'incoming'
    start?: string
    reach?: string
    targetSize?: string
    broken?: boolean
  } = $props()
</script>

<span
  class="tree-connector"
  class:tree-connector--broken={broken}
  style:--tree-start={start}
  style:--tree-reach={reach}
  style:--tree-target-size={targetSize}
  aria-hidden="true"
>
  {#if kind === 'stem' || kind === 'passthrough' || kind === 'tee'}
    <span class="seg" class:gutter__stem={kind === 'stem'}></span>
  {/if}
  {#if kind === 'tee' || kind === 'elbow' || kind === 'arm'}
    <span class="seg__arm" class:seg__arm--elbow={kind === 'elbow'}></span>
  {/if}
  {#if kind === 'incoming'}
    <span class="seg seg--incoming"></span>
  {/if}
</span>

<style>
  .tree-connector {
    --tree-line-color: color-mix(in srgb, var(--color-muted) 50%, transparent);
    position: absolute;
    inset: 0;
    pointer-events: none;
  }
  .seg {
    position: absolute;
    top: 0;
    bottom: 0;
    left: var(--tree-start);
    width: 1px;
    transform: translateX(-50%);
    background-color: var(--tree-line-color);
  }
  .gutter__stem {
    top: calc(50% + var(--tree-target-size) / 2);
  }
  .seg--incoming {
    top: calc(50% - var(--waterfall-gutter-col));
    bottom: calc(50% + var(--tree-target-size) / 2);
  }
  .tree-connector--broken .seg--incoming {
    background: none;
    border-left: 1px dashed var(--tree-line-color);
  }
  .tree-connector--broken .seg__arm {
    border-bottom-style: dashed;
  }
  .seg__arm {
    position: absolute;
    top: calc(50% - 0.5px);
    left: var(--tree-start);
    width: calc(var(--tree-reach) - var(--tree-target-size) / 2);
    height: 0;
    border-bottom: 1px solid var(--tree-line-color);
  }
  .seg__arm--elbow {
    top: 0;
    left: calc(var(--tree-start) - 0.5px);
    height: calc(50% + 0.5px);
    width: calc(var(--tree-reach) - var(--tree-target-size) / 2 + 0.5px);
    border-left: 1px solid var(--tree-line-color);
    border-bottom-left-radius: var(--radius-xl);
  }
</style>
