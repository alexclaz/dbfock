export type ImportedTableRows = { columns: string[]; rows: unknown[][] }

function parseCSV(contents: string): string[][] {
  const rows: string[][] = []
  let row: string[] = []
  let cell = ''
  let quoted = false

  for (let index = 0; index < contents.length; index++) {
    const character = contents[index]!
    if (quoted) {
      if (character === '"' && contents[index + 1] === '"') { cell += '"'; index++ }
      else if (character === '"') quoted = false
      else cell += character
      continue
    }
    if (character === '"') {
      if (cell) throw new Error('Invalid CSV: quotes must begin at the start of a value.')
      quoted = true
    }
    else if (character === ',') { row.push(cell); cell = '' }
    else if (character === '\n' || character === '\r') {
      if (character === '\r' && contents[index + 1] === '\n') index++
      row.push(cell)
      if (row.some((value) => value !== '')) rows.push(row)
      row = []
      cell = ''
    }
    else cell += character
  }
  if (quoted) throw new Error('Invalid CSV: an enclosed value is incomplete.')
  row.push(cell)
  if (row.some((value) => value !== '')) rows.push(row)
  return rows
}

function columnsFromRows(rows: Record<string, unknown>[]) {
  const columns: string[] = []
  const known = new Set<string>()
  for (const row of rows) for (const column of Object.keys(row)) if (!known.has(column)) {
    known.add(column)
    columns.push(column)
  }
  return columns
}

export function parseTableImport(contents: string, filename = ''): ImportedTableRows {
  const isJSON = filename.toLowerCase().endsWith('.json') || contents.trimStart().startsWith('[')
  if (isJSON) {
    let value: unknown
    try { value = JSON.parse(contents) }
    catch { throw new Error('Invalid JSON file.') }
    if (!Array.isArray(value) || value.some((row) => !row || Array.isArray(row) || typeof row !== 'object')) throw new Error('JSON must be an array of objects.')
    const rows = value as Record<string, unknown>[]
    const columns = columnsFromRows(rows)
    return { columns, rows: rows.map((row) => columns.map((column) => row[column] ?? null)) }
  }

  const [headers, ...rows] = parseCSV(contents)
  if (headers?.[0]) headers[0] = headers[0].replace(/^\uFEFF/, '')
  if (!headers?.length || headers.some((column) => !column.trim())) throw new Error('CSV must include a header row.')
  if (new Set(headers).size !== headers.length) throw new Error('CSV headers must be unique.')
  if (rows.some((row) => row.length !== headers.length)) throw new Error('Every CSV row must have the same number of values as its header.')
  return { columns: headers, rows }
}

function quoteIdentifier(identifier: string) {
  return `\`${identifier.replaceAll('`', '``')}\``
}

function sqlValue(value: unknown) {
  if (value === null || value === undefined) return 'NULL'
  if (typeof value === 'number') {
    if (!Number.isFinite(value)) throw new Error('Numbers in the import must be finite.')
    return String(value)
  }
  if (typeof value === 'boolean') return value ? 'TRUE' : 'FALSE'
  const text = typeof value === 'string' ? value : JSON.stringify(value)
  return `'${text.replaceAll("'", "''")}'`
}

function valueForColumn(value: unknown, databaseType?: string) {
  if (typeof value !== 'string') return value
  const type = databaseType?.toLowerCase()
  if (type === 'date' && /^\d{4}-\d{2}-\d{2}t/i.test(value)) return value.slice(0, 10)
  if ((type === 'datetime' || type === 'timestamp') && /^\d{4}-\d{2}-\d{2}t\d{2}:\d{2}:\d{2}/i.test(value)) return value.replace('T', ' ').replace(/Z$/i, '')
  return value
}

export function tableInsertStatements(database: string, table: string, columns: string[], rows: unknown[][], maxLength = 80_000, columnTypes: Record<string, string> = {}, ignoreDuplicates = false) {
  if (!columns.length || !rows.length) return []
  const prefix = `INSERT${ignoreDuplicates ? ' IGNORE' : ''} INTO ${quoteIdentifier(database)}.${quoteIdentifier(table)} (${columns.map(quoteIdentifier).join(', ')}) VALUES `
  const statements: string[] = []
  let values: string[] = []
  let length = prefix.length + 1
  for (const row of rows) {
    const item = `(${row.map((value, index) => sqlValue(valueForColumn(value, columnTypes[columns[index] ?? '']))).join(', ')})`
    if (prefix.length + item.length > maxLength) throw new Error('A row in the import is too large.')
    if (values.length && length + item.length + 2 > maxLength) {
      statements.push(`${prefix}${values.join(', ')}`)
      values = []
      length = prefix.length + 1
    }
    values.push(item)
    length += item.length + 2
  }
  if (values.length) statements.push(`${prefix}${values.join(', ')}`)
  return statements
}
