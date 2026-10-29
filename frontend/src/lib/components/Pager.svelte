<script lang="ts">
  export let page = 0;
  export let total = 0;
  export let size = 50;
  $: pages = Math.max(1, Math.ceil(total / size));
</script>

{#if total > size}
  <nav class="activity-pagination" aria-label="Results pagination">
    <span
      >{(page * size + 1).toLocaleString()}–{Math.min(
        total,
        (page + 1) * size
      ).toLocaleString()} of {total.toLocaleString()}</span
    >
    <div>
      <button
        class="btn btn-secondary btn-sm"
        disabled={page === 0}
        on:click={() => (page = Math.max(0, page - 1))}>Previous</button
      ><span>Page {page + 1} of {pages}</span><button
        class="btn btn-secondary btn-sm"
        disabled={page + 1 >= pages}
        on:click={() => (page = Math.min(pages - 1, page + 1))}>Next</button
      >
    </div>
  </nav>
{/if}
