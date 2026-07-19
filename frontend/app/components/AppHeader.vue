<script setup lang="ts">
defineProps<{ connectionName?: string; connected?: boolean }>()
defineEmits<{ add: []; toggleTheme: [] }>()
const workspace = useWorkspaceStore()
const { t } = useI18n()
function openSettings() { workspace.openTab({ id: 'settings', title: t('settings.title'), type: 'settings' }) }
</script>
<template>
  <header class="flex h-14 items-center justify-between border-b border-line bg-panel px-4">
    <div class="flex items-center gap-3"><img class="h-7 w-7 rounded-lg border border-line bg-white object-contain p-0.5" src="/branding/favicon/android-chrome-192x192.png" alt="DBfock" /><span class="font-semibold tracking-tight">DBfock</span><span class="hidden text-xs text-muted sm:block">{{ t('header.workspace') }}</span></div>
    <div class="flex items-center gap-3"><div v-if="connectionName" class="hidden items-center gap-2 rounded-md border border-line px-2.5 py-1 text-xs md:flex"><i :class="connected ? 'bg-emerald-500' : 'bg-muted'" class="h-1.5 w-1.5 rounded-full" />{{ connectionName }}</div><button class="focus-ring rounded-md px-2 py-1.5 text-sm text-muted hover:bg-canvas" :title="t('header.toggleTheme')" @click="$emit('toggleTheme')">◐</button><button class="focus-ring rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-white hover:bg-accent/90" @click="openSettings">⚙ {{ t('settings.title') }}</button></div>
  </header>
</template>
