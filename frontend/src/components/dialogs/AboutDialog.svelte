<script lang="ts">
  import Modal from './Modal.svelte'
  import Icon from '../Icon.svelte'
  import { backend } from '../../services/backend'
  import { t } from '../../services/i18n'
  import { aboutOpen } from '../../stores/ui'
  import logoUrl from '../../../../build/appicon.png'

  const VERSION = '0.2.1a'
  const GITHUB_URL = 'https://github.com/junet-1'

  function close(): void {
    aboutOpen.set(false)
  }

  function openGitHub(): void {
    backend.openExternalURL(GITHUB_URL)
  }
</script>

{#if $aboutOpen}
  <Modal titleText={$t('about.title')} on:close={close} width={420}>
    <div class="about">
      <img class="logo" src={logoUrl} alt="SSH First logo" width="112" height="112" />

      <div class="identity">
        <h2>SSH First</h2>
        <span class="version">{$t('about.version', { version: VERSION })}</span>
      </div>

      <p class="description">{$t('about.description')}</p>

      <dl>
        <div class="detail-row">
          <dt><Icon name="code" size={14} /> GitHub</dt>
          <dd>
            <a href={GITHUB_URL} on:click|preventDefault={openGitHub}>github.com/junet-1 <span aria-hidden="true">↗</span></a>
          </dd>
        </div>
        <div class="detail-row">
          <dt><Icon name="shield" size={14} /> {$t('about.license')}</dt>
          <dd>MIT License</dd>
        </div>
      </dl>
    </div>
  </Modal>
{/if}

<style>
  .about {
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 8px 10px 4px;
    text-align: center;
  }

  .logo {
    display: block;
    width: 112px;
    height: 112px;
    object-fit: contain;
    margin-bottom: 2px;
  }

  .identity {
    display: flex;
    align-items: baseline;
    justify-content: center;
    gap: 9px;
  }

  h2 {
    margin: 0;
    color: var(--text-color);
    font-size: 22px;
    font-weight: 650;
    letter-spacing: -0.02em;
  }

  .version {
    padding: 1px 6px;
    font-family: 'JetBrains Mono', ui-monospace, monospace;
    font-size: 10.5px;
    font-variant-numeric: tabular-nums;
  }

  .description {
    max-width: 310px;
    margin: 8px 0 18px;
    color: var(--text-color-secondary);
    font-size: 12.5px;
    line-height: 1.45;
  }

  dl {
    width: 100%;
    margin: 0;
    border-top: 1px solid var(--separator-color);
  }

  .detail-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 18px;
    min-height: 38px;
    border-bottom: 1px solid var(--separator-color);
    font-size: 12px;
    text-align: left;
  }

  .detail-row:last-child {
    border-bottom: none;
  }

  dt {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    color: var(--text-color-secondary);
  }

  dd {
    margin: 0;
    color: var(--text-color);
    font-weight: 500;
  }

  a {
    color: var(--accent-color);
    text-decoration: none;
  }

  a:hover {
    text-decoration: underline;
  }

  a:focus-visible {
    border-radius: 2px;
  }
</style>
