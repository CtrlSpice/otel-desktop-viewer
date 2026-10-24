<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { PaintBrush01Icon } from '@hugeicons/core-free-icons'
  import InkPicker from './InkPicker.svelte'
  import { ACCENT_INKS, restoreAccentInk } from '@/themes/accent-inks'
  import { accentInk } from '@/state/accent-ink.svelte'

  const chosen = $derived(ACCENT_INKS.find(ink => ink.id === accentInk.value)!)
  const preview = $derived(
    chosen.swatches.filter((_, index) => index % 3 === 0)
  )
</script>

<InkPicker
  label={`Accent ink: ${chosen.name}\n(${chosen.english})`}
  triggerClass="btn btn-ghost btn-sm chart-header-control theme-ink tooltip tooltip-bottom md:tooltip-left before:whitespace-pre-line"
  options={ACCENT_INKS}
  value={accentInk.value}
  onchange={id => accentInk.set(restoreAccentInk(id))}
>
  {#snippet triggerContent()}
    <HugeiconsIcon
      icon={PaintBrush01Icon}
      size={16}
      strokeWidth={1.5}
      class="shrink-0"
      aria-hidden="true"
    />
    <span>{chosen.name}</span>
    <span class="flex shrink-0 items-center gap-2" aria-hidden="true">
      {#each preview as swatch (swatch.id)}
        <span
          class="size-2 shrink-0 rounded-full"
          style:background-color={swatch.color}
        ></span>
      {/each}
    </span>
  {/snippet}
</InkPicker>
