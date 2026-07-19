import type { QueryResult } from '~/types/database'

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
