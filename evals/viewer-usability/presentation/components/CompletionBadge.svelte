<script lang="ts">
  import type { gradingStatus } from './model'

  let {
    status,
    className = 'grading-status-badge',
  }: {
    status: ReturnType<typeof gradingStatus>
    className?: string
  } = $props()

  const labels = {
    'not-started': 'Not Started',
    'in-progress': 'In Progress',
    done: 'Completed',
    'needs-attention': 'Needs Attention',
  }
</script>

<span class="badge badge-soft result-badge {className} grading-status-{status}"
  >{labels[status]}</span
>
