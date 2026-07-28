const versionManifestURL = 'https://raw.githubusercontent.com/alexclaz/dbfock/main/backend/version.txt'
const updateCommand = 'curl -fsSL https://raw.githubusercontent.com/alexclaz/dbfock/main/install.sh | bash'

type AppVersionResponse = { version: string }
export type AppUpdate = { currentVersion: string; latestVersion: string }

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

  async function checkForUpdate(force = false) {
    if (checking.value || (checked.value && !force)) return update.value
    checking.value = true
    try {
      const api = useApi()
      const installed = await api<AppVersionResponse>('/app/version')
      currentVersion.value = installed.version.trim()
      if (!versionParts(currentVersion.value)) return undefined

      const response = await fetch(versionManifestURL, { cache: 'no-store' })
      if (!response.ok) return undefined
      const latestVersion = (await response.text()).trim()
      update.value = isNewerVersion(latestVersion, currentVersion.value) ? { currentVersion: currentVersion.value, latestVersion } : undefined
      return update.value
    } catch {
      // Update checks must never interrupt normal app usage.
      return undefined
    } finally {
      checking.value = false
      checked.value = true
    }
  }

  async function copyUpdateCommand() {
    if (!import.meta.client) return false
    try {
      await navigator.clipboard.writeText(updateCommand)
      return true
    } catch {
      const input = document.createElement('textarea')
      input.value = updateCommand
      input.setAttribute('readonly', '')
      input.style.position = 'fixed'
      input.style.opacity = '0'
      document.body.append(input)
      input.select()
      const copied = document.execCommand('copy')
      input.remove()
      return copied
    }
  }

  return { update, currentVersion, checking, checkForUpdate, copyUpdateCommand }
}
