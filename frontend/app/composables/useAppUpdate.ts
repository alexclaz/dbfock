const versionManifestURL = 'https://raw.githubusercontent.com/alexclaz/dbfock/main/backend/version.txt'
const releasesURL = 'https://github.com/alexclaz/dbfock/releases/latest'

type AppVersionResponse = { version: string }
export type AppUpdate = { currentVersion: string; latestVersion: string }
export type UpdateCheckStatus = 'idle' | 'available' | 'up-to-date' | 'error'

type WailsRuntime = { BrowserOpenURL?: (url: string) => void }

function wailsRuntime() {
  if (!import.meta.client) return undefined
  return (window as Window & { runtime?: WailsRuntime }).runtime
}

function versionParts(value: string): number[] | undefined {
  const match = value.trim().match(/^v?(\d+)\.(\d+)\.(\d+)(?:[-+][0-9A-Za-z.-]+)?$/)
  if (!match) return undefined
  return match.slice(1, 4).map(Number)
}

export function isNewerVersion(candidate: string, current: string) {
  const candidateParts = versionParts(candidate)
  const currentParts = versionParts(current)
  if (!candidateParts || !currentParts) return false
  for (let index = 0; index < candidateParts.length; index++) {
    if (candidateParts[index] !== currentParts[index]) return candidateParts[index]! > currentParts[index]!
  }
  return false
}

export function useAppUpdate() {
  const update = useState<AppUpdate | undefined>('app-update', () => undefined)
  const currentVersion = useState('app-current-version', () => '')
  const checking = useState('app-update-checking', () => false)
  const checked = useState('app-update-checked', () => false)
  const checkStatus = useState<UpdateCheckStatus>('app-update-check-status', () => 'idle')
  const isWails = Boolean(wailsRuntime()?.BrowserOpenURL)

  async function checkForUpdate(force = false) {
    if (checking.value || (checked.value && !force)) return update.value
    checking.value = true
    try {
      const api = useApi()
      const installed = await api<AppVersionResponse>('/app/version')
      currentVersion.value = installed.version.trim()
      if (!versionParts(currentVersion.value)) {
        checkStatus.value = 'error'
        return undefined
      }

      const response = await fetch(versionManifestURL, { cache: 'no-store' })
      if (!response.ok) {
        checkStatus.value = 'error'
        return undefined
      }
      const latestVersion = (await response.text()).trim()
      update.value = isNewerVersion(latestVersion, currentVersion.value) ? { currentVersion: currentVersion.value, latestVersion } : undefined
      checkStatus.value = update.value ? 'available' : 'up-to-date'
      return update.value
    } catch {
      checkStatus.value = 'error'
      return undefined
    } finally {
      checking.value = false
      checked.value = true
    }
  }

  function openWailsUpdate() {
    try {
      const runtime = wailsRuntime()
      if (!runtime?.BrowserOpenURL) return false
      runtime.BrowserOpenURL(releasesURL)
      return true
    } catch {
      return false
    }
  }

  return { update, currentVersion, checking, checkStatus, isWails, checkForUpdate, openWailsUpdate }
}
