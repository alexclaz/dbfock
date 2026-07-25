<script setup lang="ts">
import type { Connection, DatabaseInfo, DatabaseUser, DatabaseUserInput } from '~/types/database'

const props = defineProps<{ connection: Connection; databases: DatabaseInfo[] }>()
const api = useApi()
const { t } = useI18n()
const { success: notifySuccess, error: notifyError } = useToast()

const users = ref<DatabaseUser[]>()
const error = ref('')
const loading = ref(false)
const saving = ref(false)
const showModal = ref(false)
const selectedKey = ref('')
const confirmingDelete = ref(false)
const form = reactive<DatabaseUserInput>({ username: '', host: '%', password: '', privileges: ['SELECT'], databases: [] })

const privileges = [
  'SELECT', 'INSERT', 'UPDATE', 'DELETE', 'CREATE', 'ALTER', 'DROP',
  'INDEX', 'REFERENCES', 'EXECUTE', 'CREATE VIEW', 'SHOW VIEW', 'TRIGGER',
]
const selectedUser = computed(() => users.value?.find((user) => `${user.username}@${user.host}` === selectedKey.value))
const account = computed(() => selectedUser.value ? `${selectedUser.value.username}@${selectedUser.value.host}` : '')
const allPrivilegesSelected = computed({
  get: () => privileges.every((privilege) => form.privileges.includes(privilege)),
  set: (selected: boolean) => { form.privileges = selected ? [...privileges] : [] },
})
function errorMessage(cause: unknown) { return cause instanceof Error ? cause.message : '' }
function accessibleDatabases(user: DatabaseUser) {
  const grants = user.grants.filter((grant) => !/^GRANT\s+USAGE\s+ON\s+\*\.\*/i.test(grant)).join('\n')
  if (/\bON\s+\*\.\*/i.test(grants)) return [t('connectionUsers.allDatabases')]
  return [...new Set([...grants.matchAll(/\bON\s+`([^`]+)`\.\*/gi)].map((match) => match[1]!))]
}

function resetForm() {
  Object.assign(form, { username: '', host: '%', password: '', privileges: ['SELECT'], databases: [] })
}
function grantsToForm(user: DatabaseUser) {
  const grants = user.grants.join(' ').toUpperCase()
  form.privileges = /\bALL PRIVILEGES\b/.test(grants) ? [...privileges] : privileges.filter((privilege) => new RegExp(`(?:GRANT |, )${privilege.replace(' ', '\\s+')}\\b`).test(grants))
  const grantsWithAccess = user.grants.filter((grant) => !/^GRANT\s+USAGE\s+ON\s+\*\.\*/i.test(grant)).join('\n')
  form.databases = /\bON\s+\*\.\*/i.test(grantsWithAccess) ? [] : [...new Set([...grantsWithAccess.matchAll(/\bON\s+`([^`]+)`\.\*/gi)].map((match) => match[1]!))]
  form.password = ''
}
async function loadUsers() {
  if (props.connection.status !== 'connected') { users.value = undefined; return }
  loading.value = true
  error.value = ''
  try {
    users.value = await api<DatabaseUser[]>(`/connections/${props.connection.id}/users`)
    if (selectedKey.value && !users.value.some((user) => `${user.username}@${user.host}` === selectedKey.value)) selectedKey.value = ''
  } catch (cause: unknown) { error.value = errorMessage(cause) || t('connectionUsers.loadError') }
  finally { loading.value = false }
}
function selectUser(user: DatabaseUser) {
  selectedKey.value = `${user.username}@${user.host}`
  confirmingDelete.value = false
  grantsToForm(user)
  showModal.value = true
}
function startCreate() {
  resetForm()
  selectedKey.value = ''
  confirmingDelete.value = false
  showModal.value = true
}
function closeModal() { showModal.value = false; selectedKey.value = ''; confirmingDelete.value = false }
async function createUser() {
  saving.value = true
  try {
    await api(`/connections/${props.connection.id}/users`, { method: 'POST', body: form })
    notifySuccess(t('connectionUsers.created'))
    await loadUsers()
    const created = users.value?.find((user) => `${user.username}@${user.host}` === `${form.username}@${form.host}`)
    if (created) selectUser(created)
  } catch (cause: unknown) { notifyError(errorMessage(cause)) }
  finally { saving.value = false }
}
async function savePermissions() {
  const user = selectedUser.value
  if (!user) return
  saving.value = true
  try {
    await api(`/connections/${props.connection.id}/users/${encodeURIComponent(user.username)}/${encodeURIComponent(user.host)}`, { method: 'PUT', body: form })
    notifySuccess(t('connectionUsers.updated'))
    await loadUsers()
  } catch (cause: unknown) { notifyError(errorMessage(cause)) }
  finally { saving.value = false }
}
async function deleteUser() {
  const user = selectedUser.value
  if (!user) return
  saving.value = true
  try {
    await api(`/connections/${props.connection.id}/users/${encodeURIComponent(user.username)}/${encodeURIComponent(user.host)}`, { method: 'DELETE' })
    notifySuccess(t('connectionUsers.deleted'))
    selectedKey.value = ''
    confirmingDelete.value = false
    showModal.value = false
    await loadUsers()
  } catch (cause: unknown) { notifyError(errorMessage(cause)) }
  finally { saving.value = false }
}

onMounted(loadUsers)
watch(() => [props.connection.id, props.connection.status], () => { selectedKey.value = ''; showModal.value = false; loadUsers() })
</script>

<template>
  <div>
    <div class="flex flex-wrap items-start justify-between gap-4"><div><h2 class="text-base font-semibold">{{ t('connectionUsers.title') }}</h2><p class="mt-1 text-sm text-muted">{{ t('connectionUsers.description') }}</p></div><button type="button" class="rounded-md bg-accent px-3 py-2 text-sm font-medium text-white" :disabled="connection.status !== 'connected'" @click="startCreate"><Icon name="lucide:user-plus" class="mr-1 inline h-4 w-4" />{{ t('connectionUsers.add') }}</button></div>
    <p v-if="connection.status !== 'connected'" class="mt-5 rounded-md border border-line bg-canvas px-3 py-2 text-sm text-muted">{{ t('connectionUsers.connectPrompt') }}</p><p v-else-if="error" class="mt-5 rounded-md border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-600">{{ error }}</p><div v-else-if="loading && !users" class="grid h-40 place-items-center text-sm text-muted">{{ t('connectionUsers.loading') }}</div>
    <div v-else class="mt-5 overflow-hidden rounded-lg border border-line bg-panel"><div class="scrollbar overflow-auto"><table class="min-w-full text-left text-sm"><thead class="bg-panel text-xs uppercase tracking-wide text-muted"><tr><th class="border-b border-line px-4 py-3 font-medium">{{ t('connectionUsers.user') }}</th><th class="border-b border-line px-4 py-3 font-medium">{{ t('connectionUsers.databases') }}</th><th class="border-b border-line px-4 py-3 font-medium">{{ t('connectionUsers.authentication') }}</th><th class="border-b border-line px-4 py-3 font-medium">{{ t('connectionUsers.status') }}</th><th class="w-12 border-b border-line px-4 py-3" /></tr></thead><tbody><tr v-for="user in users" :key="`${user.username}@${user.host}`" class="cursor-pointer border-b border-line last:border-b-0 hover:bg-canvas" @click="selectUser(user)"><td class="px-4 py-3"><p class="font-medium">{{ user.username }}</p><p class="mt-0.5 font-mono text-xs text-muted">@{{ user.host }}</p></td><td class="px-4 py-3"><div v-if="accessibleDatabases(user).length" class="flex max-w-xs flex-wrap gap-1"><span v-for="database in accessibleDatabases(user)" :key="database" class="rounded bg-accent/10 px-1.5 py-0.5 font-mono text-xs text-accent">{{ database }}</span></div><span v-else class="text-xs text-muted">—</span></td><td class="px-4 py-3 text-muted">{{ user.authPlugin || '—' }}</td><td class="px-4 py-3"><span v-if="user.locked || user.passwordExpired" class="rounded bg-amber-500/15 px-2 py-1 text-xs text-amber-700">{{ user.locked ? t('connectionUsers.locked') : t('connectionUsers.expired') }}</span><span v-else class="rounded bg-emerald-500/15 px-2 py-1 text-xs text-emerald-700">{{ t('connectionUsers.active') }}</span></td><td class="px-4 py-3 text-right"><button type="button" class="grid h-7 w-7 place-items-center rounded text-muted hover:bg-line" :aria-label="t('tree.edit')" @click.stop="selectUser(user)"><Icon name="lucide:pencil" class="h-3.5 w-3.5" /></button></td></tr></tbody></table></div><p v-if="!users?.length" class="px-4 py-10 text-center text-sm text-muted">{{ t('connectionUsers.empty') }}</p></div>
    <Teleport to="body"><div v-if="showModal" class="fixed inset-0 z-50 grid place-items-center bg-slate-950/40 p-4" @mousedown.self="closeModal"><form class="scrollbar max-h-[calc(100vh-2rem)] w-full max-w-xl overflow-auto rounded-xl border border-line bg-panel p-5 shadow-panel" @submit.prevent="selectedUser ? savePermissions() : createUser()"><div class="flex items-start justify-between gap-4"><div><h3 class="font-semibold">{{ selectedUser ? t('tree.edit') : t('connectionUsers.add') }}</h3><p v-if="selectedUser" class="mt-1 font-mono text-xs text-muted">{{ account }}</p><p v-else class="mt-1 text-sm text-muted">{{ t('connectionUsers.addDescription') }}</p></div><button type="button" class="grid h-7 w-7 place-items-center rounded text-muted hover:bg-canvas" @click="closeModal"><Icon name="lucide:x" class="h-4 w-4" /></button></div><div v-if="!selectedUser" class="mt-5 grid gap-4 sm:grid-cols-2"><label class="grid gap-1.5 text-sm font-medium">{{ t('connectionUsers.user') }}<input v-model="form.username" required class="field" autocomplete="off" placeholder="app_user"></label><label class="grid gap-1.5 text-sm font-medium">{{ t('connectionUsers.host') }}<input v-model="form.host" required class="field" autocomplete="off" placeholder="%"></label><label class="grid gap-1.5 text-sm font-medium sm:col-span-2">{{ t('connectionUsers.password') }}<input v-model="form.password" required type="password" class="field" autocomplete="new-password"></label></div><template v-else><label class="mt-5 grid gap-1.5 text-sm font-medium">{{ t('connectionUsers.newPassword') }}<input v-model="form.password" type="password" class="field" autocomplete="new-password" :placeholder="t('connectionUsers.passwordHint')"></label><div class="mt-5"><p class="text-xs font-medium text-muted">{{ t('connectionUsers.currentPermissions') }}</p><ul class="mt-2 space-y-1 rounded-md bg-canvas p-3 text-xs text-muted"><li v-for="grant in selectedUser.grants" :key="grant" class="break-words font-mono">{{ grant }}</li></ul></div></template><div class="mt-5 border-t border-line pt-5"><label class="grid gap-1.5 text-sm font-medium">{{ t('connectionUsers.scope') }}<AppMultiSelect v-model="form.databases" :options="databases.map((database) => ({ value: database.name, label: database.name }))" :placeholder="t('connectionUsers.allDatabases')" /></label><p class="mt-1 text-xs text-muted">{{ t('connectionUsers.multiScopeDescription') }}</p><p v-if="selectedUser" class="mt-2 text-xs text-amber-700">{{ t('connectionUsers.updateWarning') }}</p><div class="mt-5 flex items-center justify-between"><p class="text-xs font-medium text-muted">{{ t('connectionUsers.permissions') }}</p><label class="flex items-center gap-2 text-xs font-medium"><input v-model="allPrivilegesSelected" type="checkbox" class="accent-[var(--accent)]"><span>{{ t('connectionUsers.allPermissions') }}</span></label></div><div class="mt-2 grid grid-cols-2 gap-2 sm:grid-cols-3"><label v-for="privilege in privileges" :key="privilege" class="flex items-center gap-2 text-xs"><input v-model="form.privileges" type="checkbox" :value="privilege" class="accent-[var(--accent)]"><span>{{ privilege }}</span></label></div></div><div class="mt-6 flex items-center justify-between gap-3 border-t border-line pt-4"><div v-if="selectedUser"><template v-if="confirmingDelete"><p class="mb-2 text-xs text-rose-600">{{ t('connectionUsers.deleteConfirm', { name: account }) }}</p><div class="flex gap-2"><button type="button" class="rounded-md border border-line px-2.5 py-1.5 text-xs" @click="confirmingDelete = false">{{ t('connection.cancel') }}</button><button type="button" class="rounded-md bg-rose-600 px-2.5 py-1.5 text-xs font-medium text-white" :disabled="saving" @click="deleteUser">{{ t('connectionUsers.delete') }}</button></div></template><button v-else type="button" class="text-sm text-rose-600 hover:underline" @click="confirmingDelete = true">{{ t('connectionUsers.delete') }}</button></div><span v-else /><div class="flex gap-2"><button type="button" class="rounded-md px-3 py-2 text-sm hover:bg-canvas" @click="closeModal">{{ t('connection.cancel') }}</button><button type="submit" class="rounded-md bg-accent px-3 py-2 text-sm font-medium text-white disabled:opacity-60" :disabled="saving">{{ saving ? t('common.saving') : selectedUser ? t('common.save') : t('connectionUsers.create') }}</button></div></div></form></div></Teleport>
  </div>
</template>

<style scoped>
.field { @apply h-10 rounded-md border border-line bg-canvas px-3 text-sm font-normal text-ink shadow-sm focus:outline-none focus:ring-2 focus:ring-accent/50; }
</style>
