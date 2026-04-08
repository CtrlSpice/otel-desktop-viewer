<script lang="ts">
  /** Globe / world (24×24), from Huge Icons. */
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
  <g stroke="currentColor" stroke-width="1.5">
    <path
      d="M22 12c0-5.523-4.477-10-10-10S2 6.477 2 12s4.477 10 10 10s10-4.477 10-10Z"
    />
    <path
      stroke-linejoin="round"
      d="M20 5.699c-.935.067-2.132.43-2.962 1.504c-1.5 1.94-2.999 2.103-3.999 1.456c-1.5-.97-.239-2.543-1.999-3.397C9.893 4.705 9.733 3.19 10.372 2M2 11c.763.662 1.83 1.268 3.089 1.268c2.6 0 3.12.497 3.12 2.484s0 1.987.52 3.477c.338.97.456 1.938-.218 2.771M22 13.452c-.887-.51-2-.721-3.127.088c-2.155 1.55-3.642.266-4.311 1.549C13.577 16.977 17.096 17.57 14 22"
    />
  </g>
</svg>
