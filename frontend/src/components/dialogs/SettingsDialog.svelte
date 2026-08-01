<script lang="ts">
  import Modal from './Modal.svelte'
  import { t, locale, type Locale } from '../../services/i18n'
  import { backend } from '../../services/backend'
  import {
    applyTheme,
    MAX_TERMINAL_FONT_SIZE,
    MIN_TERMINAL_FONT_SIZE,
    normalizeTerminalFontSize,
    settingsOpen,
    terminalFontSize,
    theme,
    type ThemePreference
  } from '../../stores/ui'

  function close(): void {
    settingsOpen.set(false)
  }

  async function onThemeChange(value: string): Promise<void> {
    const pref = value as ThemePreference
    theme.set(pref)
    applyTheme(pref)
    await backend.setSetting('theme', pref)
  }

  async function onLocaleChange(value: string): Promise<void> {
    const l = value as Locale
    locale.set(l)
    await backend.setSetting('locale', l)
  }

  async function onTerminalFontSizeChange(value: number): Promise<void> {
    const size = normalizeTerminalFontSize(value)
    terminalFontSize.set(size)
    await backend.setSetting('terminalFontSize', String(size))
  }
</script>

{#if $settingsOpen}
  <Modal titleText={$t('settings.title')} on:close={close} width={360}>
    <div class="content">
      <section>
        <h4>{$t('settings.appearance')}</h4>
        <label>
          <span>{$t('settings.theme')}</span>
          <select value={$theme} on:change={(e) => onThemeChange(e.currentTarget.value)}>
            <option value="system">{$t('settings.theme.system')}</option>
            <option value="light">{$t('settings.theme.light')}</option>
            <option value="dark">{$t('settings.theme.dark')}</option>
          </select>
        </label>
        <label>
          <span>{$t('settings.terminalFontSize')}</span>
          <input
            class="font-size"
            type="number"
            min={MIN_TERMINAL_FONT_SIZE}
            max={MAX_TERMINAL_FONT_SIZE}
            value={$terminalFontSize}
            on:change={(e) => onTerminalFontSizeChange(e.currentTarget.valueAsNumber)}
          />
        </label>
      </section>

      <section>
        <h4>{$t('settings.language')}</h4>
        <label>
          <span>{$t('settings.language')}</span>
          <select value={$locale} on:change={(e) => onLocaleChange(e.currentTarget.value)}>
            <option value="en">{$t('settings.language.en')}</option>
            <option value="de">{$t('settings.language.de')}</option>
          </select>
        </label>
      </section>
    </div>

    <svelte:fragment slot="footer">
      <button class="primary" on:click={close}>{$t('settings.close')}</button>
    </svelte:fragment>
  </Modal>
{/if}

<style>
  .content {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  h4 {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    color: var(--text-color-secondary);
    margin: 0 0 6px;
  }

  label {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    font-size: 12.5px;
  }

  select {
    min-width: 140px;
  }

  .font-size {
    width: 72px;
    text-align: right;
  }
</style>
