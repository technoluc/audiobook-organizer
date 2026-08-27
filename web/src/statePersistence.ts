type PersistedWebState = {
  sourceFolder?: string
  outputFolder?: string
  metadataSource?: string
  absUrl?: string
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
    // Persistence is a convenience only; never break the UI when storage is unavailable.
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
  let absInputs = [...document.querySelectorAll<HTMLInputElement>('input[aria-label="ABS path prefix"]')]
  let localInputs = [...document.querySelectorAll<HTMLInputElement>('input[aria-label="Local path prefix"]')]

  while (addButton && absInputs.length < mappings.length) {
    addButton.click()
    absInputs = [...document.querySelectorAll<HTMLInputElement>('input[aria-label="ABS path prefix"]')]
    localInputs = [...document.querySelectorAll<HTMLInputElement>('input[aria-label="Local path prefix"]')]
  }

  mappings.forEach((mapping, index) => {
    setInputValue(absInputs[index] ?? null, mapping.absPrefix)
    setInputValue(localInputs[index] ?? null, mapping.localPrefix)
  })
}

function collectState(): PersistedWebState {
  const checkedMetadataButton = document.querySelector<HTMLButtonElement>(
    '.metadata-source-control button[role="radio"][aria-checked="true"]',
  )
  const absInputs = [...document.querySelectorAll<HTMLInputElement>('input[aria-label="ABS path prefix"]')]
  const localInputs = [...document.querySelectorAll<HTMLInputElement>('input[aria-label="Local path prefix"]')]

  return {
    sourceFolder: inputByLabel('Source folder')?.value,
    outputFolder: inputByLabel('Output folder')?.value,
    metadataSource: checkedMetadataButton ? buttonText(checkedMetadataButton) : undefined,
    absUrl: inputByLabel('ABS server URL')?.value,
    absLibrary: selectByLabel('ABS library')?.value,
    absPathMappings: absInputs.map((input, index) => ({
      absPrefix: input.value,
      localPrefix: localInputs[index]?.value ?? '',
    })),
    layout: selectByLabel('Layout')?.value,
  }
}

export function installWebStatePersistence() {
  if (typeof window === 'undefined') return

  const persisted = readState()
  let restoring = false
  let saveTimer: number | undefined

  const restore = () => {
    restoring = true
    try {
      setInputValue(inputByLabel('Source folder'), persisted.sourceFolder)
      setInputValue(inputByLabel('Output folder'), persisted.outputFolder)
      setInputValue(inputByLabel('ABS server URL'), persisted.absUrl)
      setSelectValue(selectByLabel('Layout'), persisted.layout)
      restoreMetadataSource(persisted.metadataSource)
      restoreMappings(persisted.absPathMappings)
      setSelectValue(selectByLabel('ABS library'), persisted.absLibrary)
    } finally {
      restoring = false
    }
  }

  const save = () => {
    if (restoring) return
    if (saveTimer !== undefined) window.clearTimeout(saveTimer)
    saveTimer = window.setTimeout(() => writeState(collectState()), 100)
  }

  document.addEventListener('input', save, true)
  document.addEventListener('change', save, true)
  document.addEventListener('click', save, true)

  const observer = new MutationObserver(() => {
    // Some controls (ABS libraries/mappings) appear only after async API calls.
    // Re-applying persisted values is idempotent and lets those late controls restore too.
    window.setTimeout(restore, 0)
  })
  observer.observe(document.body, { childList: true, subtree: true })

  window.setTimeout(restore, 0)
  window.setTimeout(restore, 250)
  window.setTimeout(restore, 1000)
}
