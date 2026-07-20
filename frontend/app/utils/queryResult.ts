import type { QueryResult } from '~/types/database'

export type QueryRowUpdate = { original: Record<string, unknown>; changes: Record<string, unknown> }

function valueForExport(value: unknown) {
  if (value === null || value === undefined) return ''
  if (typeof value === 'bigint') return value.toString()
  if (typeof value === 'object') return JSON.stringify(value, (_, nested) => typeof nested === 'bigint' ? nested.toString() : nested) ?? ''
  return String(value)
}

function csvCell(value: unknown) {
  const text = valueForExport(value)
  return /[",\r\n]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text
}

export function queryResultAsJSON(result?: QueryResult) {
  return JSON.stringify(result?.rows ?? [], (_, value) => typeof value === 'bigint' ? value.toString() : value, 2) ?? '[]'
}

export function queryResultAsCSV(result?: QueryResult) {
  if (!result) return ''
  const columns = result.columns ?? []
  const headers = columns.map((column) => csvCell(column.name)).join(',')
  const rows = (result.rows ?? []).map((row) => columns.map((column) => csvCell(row[column.name])).join(','))
  return [headers, ...rows].join('\r\n')
}

export function queryResultAsTSV(result?: QueryResult) {
  if (!result) return ''
  const columns = result.columns ?? []
  const headers = columns.map((column) => valueForExport(column.name)).join('\t')
  const rows = (result.rows ?? []).map((row) => columns.map((column) => valueForExport(row[column.name]).replaceAll('\t', ' ').replaceAll('\r', ' ').replaceAll('\n', ' ')).join('\t'))
  return [headers, ...rows].join('\n')
}

function sameValue(left: unknown, right: unknown) {
  if (Object.is(left, right)) return true
  try { return JSON.stringify(left) === JSON.stringify(right) }
  catch { return false }
}

export function queryResultEdits(original: QueryResult, edited: QueryResult): QueryRowUpdate[] {
  if (original.rows.length !== edited.rows.length) throw new Error('Adding or removing rows is not supported by inline editing.')
  return edited.rows.flatMap((row, index) => {
    const previous = original.rows[index]
    if (!previous) return []
    const changes = Object.fromEntries(Object.entries(row).filter(([column, value]) => !sameValue(previous[column], value)))
    return Object.keys(changes).length ? [{ original: previous, changes }] : []
  })
}
