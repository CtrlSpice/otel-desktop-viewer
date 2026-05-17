<script lang="ts">
  /** Calendar + clock (24×24). */
  type Props = {
    class?: string
  }
  let { class: className = 'h-4 w-4' }: Props = $props()
</script>

<svg
  xmlns="http://www.w3.org/2000/svg"
  viewBox="0 0 24 24"
  fill="none"
  class="shrink-0 {className}"
  aria-hidden="true"
>
  <g stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
    <rect x="3" y="4" width="18" height="18" rx="2" />
    <path d="M16 2v4M8 2v4M3 10h18" />
    <path d="M12 14v2l1.5 1.5" />
  </g>
</svg>
