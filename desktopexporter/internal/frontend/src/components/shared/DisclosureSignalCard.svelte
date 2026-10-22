<script lang="ts">
  import type { Snippet } from 'svelte'
  import SignalCard from './SignalCard.svelte'

  let {
    title,
    titleContent,
    description,
    descriptionIsID,
    date,
    timestamp,
    timestampUnit,
    timestampSuffix,
    badge,
    badgePlacement = 'end',
    reveal = false,
    identity,
    wrapTitle = false,
    children,
  }: {
    title: string
    titleContent?: Snippet
    description?: string
    descriptionIsID?: boolean
    date?: Snippet
    timestamp?: string
    timestampUnit?: string
    timestampSuffix?: string
    badge?: Snippet
    badgePlacement?: 'inline' | 'end'
    reveal?: boolean
    /** Stable record identity; a different record starts closed without remounting. */
    identity?: string
    wrapTitle?: boolean
    children: Snippet
  } = $props()

  const contentID = $props.id()
  let open = $state(false)

  $effect(() => {
    void identity
    open = false
  })

  $effect(() => {
    void identity
    if (reveal) open = true
  })
</script>

<div class="min-w-0 w-full">
  <SignalCard
    id={contentID}
    {title}
    {titleContent}
    {wrapTitle}
    {description}
    {descriptionIsID}
    {date}
    {timestamp}
    {timestampUnit}
    {timestampSuffix}
    timeLayout="labeled"
    {badge}
    {badgePlacement}
    selected={reveal}
    expanded={open}
    controls={contentID}
    onclick={() => (open = !open)}
  />
  <div id={contentID} hidden={!open}>
    {@render children()}
  </div>
</div>
