import type { WorkspaceTab } from '~/types/database'

const workspaceIcons: Record<WorkspaceTab['type'], string> = {
  empty: 'lucide:circle',
  welcome: 'lucide:house',
  saved: 'lucide:bookmark',
  smart: 'lucide:sparkles',
  history: 'lucide:history',
  sql: 'lucide:file-code-2',
  table: 'lucide:table-2',
  database: 'lucide:database',
  'connection-home': 'lucide:layout-dashboard',
  settings: 'lucide:settings-2',
  stats: 'lucide:chart-no-axes-combined',
}

export function workspaceIcon(type: WorkspaceTab['type']) {
  return workspaceIcons[type]
}
