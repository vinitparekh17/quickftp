<script lang="ts">
  import Icon from './Icon.svelte';
  import type { IconName } from '../icons';
  import { createEventDispatcher } from 'svelte';

  export let icon: IconName;
  export let label: string;
  export let size = 16;
  export let variant: 'secondary' | 'ghost' | 'danger' | 'primary' = 'ghost';
  export let disabled = false;
  export let className = '';

  const dispatch = createEventDispatcher();
  function handleClick(e: MouseEvent) {
    dispatch('click', e);
  }
</script>

<button
  type="button"
  class="btn btn-{variant} btn-icon {className}"
  {disabled}
  aria-label={label}
  title={label}
  on:click={handleClick}
>
  <Icon name={icon} {size} />
</button>

