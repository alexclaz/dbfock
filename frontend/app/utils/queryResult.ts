import type { QueryResult } from '~/types/database'
import { tableInsertStatements } from '~/utils/tableTransfer'

export type QueryRowUpdate = { original: Record<string, unknown>; changes: Record<string, unknown> }
export type QueryResultEditOptions = { editableColumns?: string[]; keyColumns?: string[] }

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
  if (!result) return '[]'

  // Rows arrive from the API as objects, whose keys may have been reordered
  // during serialization. Rebuild each row from the result metadata so the
  // JSON view and export follow the schema/query column order.
  const columnNames = new Set(result.columns.map((column) => column.name))
  const rows = result.rows.map((row) => {
    const ordered: Record<string, unknown> = {}
    for (const column of result.columns) {
      if (Object.prototype.hasOwnProperty.call(row, column.name)) ordered[column.name] = row[column.name]
    }
    // Preserve values that are not represented in the metadata, without
    // changing the schema-defined order of known columns.
    for (const [name, value] of Object.entries(row)) {
      if (!columnNames.has(name)) ordered[name] = value
    }
    return ordered
  })

  return JSON.stringify(rows, (_, value) => typeof value === 'bigint' ? value.toString() : value, 2)
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

export type InsertTarget = { database: string; table: string; columns: string[] }

/** Build MySQL INSERT statements for rows selected in a query result. */
export function queryRowsAsInsert(result: QueryResult | undefined, target: InsertTarget, rows: Record<string, unknown>[]) {
  if (!result || !rows.length) return ''
  const resultColumns = new Map(result.columns.map((column) => [column.name, column]))
  const columns = target.columns.filter((column) => resultColumns.has(column))
  if (!columns.length) return ''
  const columnTypes = Object.fromEntries(columns.map((column) => [column, resultColumns.get(column)?.databaseType ?? '']))
  return tableInsertStatements(target.database, target.table, columns, rows.map((row) => columns.map((column) => row[column])), 80_000, columnTypes).map((statement) => `${statement};`).join('\n')
}

function sameValue(left: unknown, right: unknown) {
  if (Object.is(left, right)) return true
  try { return JSON.stringify(left) === JSON.stringify(right) }
  catch { return false }
}

export function queryResultEdits(original: QueryResult, edited: QueryResult, options: QueryResultEditOptions = {}): QueryRowUpdate[] {
  if (original.rows.length !== edited.rows.length) throw new Error('Adding or removing rows is not supported by inline editing.')
  const editableColumns = options.editableColumns ? new Set(options.editableColumns) : undefined
  return edited.rows.flatMap((row, index) => {
    const previous = original.rows[index]
    if (!previous) return []
    const changes = Object.fromEntries(Object.entries(row).filter(([column, value]) => (!editableColumns || editableColumns.has(column)) && !sameValue(previous[column], value)))
    const keyColumns = options.keyColumns
    const key = keyColumns ? Object.fromEntries(keyColumns.map((column) => [column, previous[column]])) : previous
    return Object.keys(changes).length ? [{ original: key, changes }] : []
  })
}
