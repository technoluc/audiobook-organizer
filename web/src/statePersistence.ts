type PersistedWebState = {
  sourceFolder?: string
  outputFolder?: string
  metadataSource?: string
  absUrl?: string
  absToken?: string
  absLibrary?: string
  absPathMappings?: Array<{ absPrefix: string; localPrefix: string }>
  layout?: string
}

const STORAGE_KEY = 'audiobook-organizer:web-state:v1'

function readState(): PersistedWebState {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    return raw ? (JSON.parse(raw) as PersistedWebState) : {}
  } catch {
    return {}
  }
}

function writeState(state: PersistedWebState) {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state))
  } catch {
    // Persistence must never break the UI if browser storage is unavailable.
  }
}

function inputByLabel(label: string): HTMLInputElement | null {
  return document.querySelector<HTMLInputElement>(`input[aria-label="${label}"]`)
}

function selectByLabel(label: string): HTMLSelectElement | null {
  return document.querySelector<HTMLSelectElement>(`select[aria-label="${label}"]`)
}

function setInputValue(input: HTMLInputElement | null, value: string | undefined) {
  if (!input || value === undefined || input.value === value) return
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  input.dispatchEvent(new Event('change', { bubbles: true }))
}

function setSelectValue(select: HTMLSelectElement | null, value: string | undefined) {
  if (!select || value === undefined || select.value === value) return
  if (![...select.options].some((option) => option.value === value)) return
  select.value = value
  select.dispatchEvent(new Event('change', { bubbles: true }))
}

function buttonText(button: Element): string {
  return (button.textContent ?? '').replace(/\s+/g, ' ').trim()
}

function restoreMetadataSource(value: string | undefined) {
  if (!value) return
  const buttons = document.querySelectorAll<HTMLButtonElement>('.metadata-source-control button[role="radio"]')
  const target = [...buttons].find((button) => buttonText(button) === value)
  if (target && target.getAttribute('aria-checked') !== 'true') target.click()
}

function restoreMappings(mappings: PersistedWebState['absPathMappings']) {
  if (!mappings?.length) return

  const addButton = [...document.querySelectorAll<HTMLButtonElement>('button')].find((button) =>
    buttonText(button).includes('Add Mapping'),
  )
  const absInputs = [...document.querySelectorAll<HTMLInputElement>('input[aria-label="ABS path prefix"]')]
  const localInputs = [...document.querySelectorAll<HTMLInputElement>('input[aria-label="Local path prefix"]')]

  // Vue renders a newly added mapping asynchronously. Add at most one row per
  // render cycle; the MutationObserver below will continue after Vue updates.
  if (addButton && absInputs.length < mappings.length) {
    addButton.click()
    return
  }

  mappings.forEach((mapping, index) => {
    setInputValue(absInputs[index] ?? null, mapping.absPrefix)
    setInputValue(localInputs[index] ?? null, mapping.localPrefix)
  })
}

function collectState(): PersistedWebState {
  const state: PersistedWebState = {}

  const sourceFolder = inputByLabel('Source folder')
  const outputFolder = inputByLabel('Output folder')
  const absUrl = inputByLabel('ABS server URL')
  const absToken = inputByLabel('ABS API token')
  const absLibrary = selectByLabel('ABS library')
  const layout = selectByLabel('Layout')
  const checkedMetadataButton = document.querySelector<HTMLButtonElement>(
    '.metadata-source-control button[role="radio"][aria-checked="true"]',
  )
  const absInputs = [...document.querySelectorAll<HTMLInputElement>('input[aria-label="ABS path prefix"]')]
  const localInputs = [...document.querySelectorAll<HTMLInputElement>('input[aria-label="Local path prefix"]')]

  if (sourceFolder) state.sourceFolder = sourceFolder.value
  if (outputFolder) state.outputFolder = outputFolder.value
  if (checkedMetadataButton) state.metadataSource = buttonText(checkedMetadataButton)
  if (absUrl) state.absUrl = absUrl.value
  if (absToken) state.absToken = absToken.value
  if (absLibrary) state.absLibrary = absLibrary.value
  if (layout) state.layout = layout.value
  if (absInputs.length > 0) {
    state.absPathMappings = absInputs.map((input, index) => ({
      absPrefix: input.value,
      localPrefix: localInputs[index]?.value ?? '',
    }))
  }

  return state
}

export function installWebStatePersistence() {
  if (typeof window === 'undefined') return

  let persisted = readState()
  let restoring = false
  let saveTimer: number | undefined

  const restoreABSControls = () => {
    // The ABS panel is conditionally rendered only after the metadata source has
    // switched to Audiobookshelf. Restore these fields whenever that panel appears.
    setInputValue(inputByLabel('ABS server URL'), persisted.absUrl)
    setInputValue(inputByLabel('ABS API token'), persisted.absToken)
    restoreMappings(persisted.absPathMappings)
    setSelectValue(selectByLabel('ABS library'), persisted.absLibrary)
  }

  const restore = () => {
    restoring = true
    try {
      setInputValue(inputByLabel('Source folder'), persisted.sourceFolder)
      setInputValue(inputByLabel('Output folder'), persisted.outputFolder)
      setSelectValue(selectByLabel('Layout'), persisted.layout)
      restoreMetadataSource(persisted.metadataSource)
      restoreABSControls()
    } finally {
      restoring = false
    }
  }

  const save = () => {
    if (restoring) return
    if (saveTimer !== undefined) window.clearTimeout(saveTimer)
    saveTimer = window.setTimeout(() => {
      persisted = { ...persisted, ...collectState() }
      writeState(persisted)
    }, 100)
  }

  document.addEventListener('input', save, true)
  document.addEventListener('change', save, true)
  document.addEventListener('click', save, true)

  // Persist immediately when navigating/reloading as an extra guard around the
  // short debounce above.
  window.addEventListener('beforeunload', () => {
    persisted = { ...persisted, ...collectState() }
    writeState(persisted)
  })

  // Vue/bootstrap can overwrite controls during startup.
  window.setTimeout(restore, 0)
  window.setTimeout(restore, 250)
  window.setTimeout(restore, 1000)
  window.setTimeout(restore, 2500)

  // ABS controls are conditionally/asynchronously rendered. Restore URL, token,
  // library and mappings whenever that subtree appears, but debounce DOM churn.
  let observerScheduled = false
  const observer = new MutationObserver(() => {
    if (observerScheduled) return
    observerScheduled = true

    window.requestAnimationFrame(() => {
      observerScheduled = false
      restoring = true
      try {
        restoreABSControls()
      } finally {
        restoring = false
      }
    })
  })
  observer.observe(document.body, { childList: true, subtree: true })
}
