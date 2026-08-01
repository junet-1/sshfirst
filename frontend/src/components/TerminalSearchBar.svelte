<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  import Icon from './Icon.svelte'
  import { t } from '../services/i18n'

  export let query = ''
  export let resultIndex = -1
  export let resultCount = 0

  const dispatch = createEventDispatcher<{
    query: string
    next: void
    previous: void
    close: void
  }>()

  let input: HTMLInputElement

  onMount(() => {
    input.focus()
    input.select()
  })

  function onInput(): void {
    dispatch('query', query)
  }

  function onKeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      event.preventDefault()
      dispatch('close')
    } else if (event.key === 'Enter') {
      event.preventDefault()
      dispatch(event.shiftKey ? 'previous' : 'next')
    }
  }
</script>

<div class="searchbar" role="search" aria-label={$t('terminalSearch.title')}>
  <Icon name="search" size={13} />
  <input
    bind:this={input}
    bind:value={query}
    aria-label={$t('terminalSearch.placeholder')}
    placeholder={$t('terminalSearch.placeholder')}
    spellcheck="false"
    on:input={onInput}
    on:keydown={onKeydown}
  />
  <span class:no-results={query && resultCount === 0} class="count">
    {#if query}{resultCount > 0 ? `${resultIndex + 1}/${resultCount}` : $t('terminalSearch.noResults')}{/if}
  </span>
  <button title={$t('terminalSearch.previous')} disabled={!query || resultCount === 0} on:click={() => dispatch('previous')}>
    <Icon name="chevron-up" size={12} />
  </button>
  <button title={$t('terminalSearch.next')} disabled={!query || resultCount === 0} on:click={() => dispatch('next')}>
    <Icon name="chevron-down" size={12} />
  </button>
  <button title={$t('terminalSearch.close')} on:click={() => dispatch('close')}>
    <Icon name="x" size={12} />
  </button>
</div>

<style>
  .searchbar {
    position: absolute;
    z-index: 12;
    top: 8px;
    right: 14px;
    height: 30px;
    display: flex;
    align-items: center;
    gap: 5px;
    padding: 0 4px 0 8px;
    color: var(--text-color-secondary);
    background: var(--window-bg);
    border: 1px solid var(--border-color);
    border-radius: 3px;
    box-shadow: 0 3px 12px var(--shadow-color);
  }

  input {
    width: 210px;
    height: 22px;
    padding: 1px 4px;
    border: none;
    background: transparent;
    color: var(--text-color);
    font-size: 12px;
    outline: none;
  }

  .count {
    min-width: 42px;
    font-size: 10.5px;
    text-align: right;
    color: var(--disabled-text-color);
  }

  .count.no-results {
    color: var(--error-color);
  }

  button {
    width: 22px;
    height: 22px;
    padding: 0;
    display: grid;
    place-items: center;
    border: none;
    background: transparent;
    color: var(--text-color-secondary);
  }

  button:hover:not(:disabled) {
    background: var(--hover-bg);
  }
</style>
