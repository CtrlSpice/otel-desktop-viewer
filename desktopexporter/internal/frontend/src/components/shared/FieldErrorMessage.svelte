<script lang="ts">
  interface Props {
    /** Stable id for aria-describedby on related inputs. */
    id?: string
    message: string
  }

  let { id, message }: Props = $props()
</script>

<p class="field-error-message" role="alert" {id}>{message}</p>
