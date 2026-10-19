<script lang="ts">
  import type { Snippet } from 'svelte'
  import SignalCard from './SignalCard.svelte'

  let {
    title,
    titleContent,
    description,
    descriptionIsID,
    timestamp,
    timestampUnit,
    timestampSuffix,
    badge,
    reveal = false,
    children,
  }: {
    title: string
    titleContent?: Snippet
    description?: string
    descriptionIsID?: boolean
    timestamp?: string
    timestampUnit?: string
    timestampSuffix?: string
    badge: Snippet
    reveal?: boolean
    children: Snippet
  } = $props()

  const contentID = $props.id()
  let open = $state(false)

  $effect(() => {
    if (reveal) open = true
  })
</script>

<div class="min-w-0 w-full">
  <SignalCard
    id={contentID}
    {title}
    {titleContent}
    {description}
    {descriptionIsID}
    {timestamp}
    {timestampUnit}
    {timestampSuffix}
    timeLayout="labeled"
    {badge}
    selected={reveal}
    expanded={open}
    controls={contentID}
    onclick={() => (open = !open)}
  />
  <div id={contentID} hidden={!open}>
    {@render children()}
  </div>
</div>
