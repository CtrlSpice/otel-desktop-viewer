<script lang="ts">
  /** Sliders / customize (24×24). */
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
  <g stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
    <path d="M21 5h-4M7 5H3" />
    <path d="M21 19h-4M7 19H3" />
    <path d="M21 12h-10M7 12H3" />
    <circle cx="9" cy="5" r="2" />
    <circle cx="9" cy="19" r="2" />
    <circle cx="15" cy="12" r="2" />
  </g>
</svg>
