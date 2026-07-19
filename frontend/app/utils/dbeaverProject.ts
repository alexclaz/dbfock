const ZIP_EOCD = 0x06054b50
const ZIP_CENTRAL_FILE = 0x02014b50
const ZIP_LOCAL_FILE = 0x04034b50
const MAX_ARCHIVE_BYTES = 25 * 1024 * 1024
const MAX_DATA_SOURCES_BYTES = 1024 * 1024

export class DBeaverProjectError extends Error {}
export class DBeaverNoMySQLConnectionsError extends DBeaverProjectError {}

export type DBeaverMySQLConnection = {
  name: string
  host: string
  port: number
  username: string
  initialDatabase: string
  sslEnabled: boolean
}

type ZipEntry = { compression: number; compressedSize: number; uncompressedSize: number; localOffset: number }
type DBeaverURL = { host: string; port?: number; database?: string; username?: string }

function asObject(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : undefined
}
function stringValue(value: unknown): string | undefined { return typeof value === 'string' && value.trim() ? value.trim() : undefined }
function portValue(value: unknown): number | undefined {
  const port = typeof value === 'number' ? value : Number(stringValue(value))
  return Number.isInteger(port) && port >= 1 && port <= 65535 ? port : undefined
}
function firstString(sources: Array<Record<string, unknown> | undefined>, keys: string[]): string | undefined {
  for (const source of sources) for (const key of keys) { const value = stringValue(source?.[key]); if (value) return value }
}
function parseMySQLURL(value: unknown): DBeaverURL | undefined {
  const jdbcURL = stringValue(value)
  if (!jdbcURL?.toLowerCase().startsWith('jdbc:mysql:')) return
  try {
    const url = new URL(jdbcURL.slice(5))
    const database = url.pathname.replace(/^\/+/, '')
    return { host: url.hostname, port: portValue(url.port), database: database ? decodeURIComponent(database) : undefined, username: stringValue(url.searchParams.get('user')) }
  } catch { return undefined }
}
function sslValue(value: unknown): boolean { return value === true || (typeof value === 'string' && ['true', '1', 'required', 'verify_ca', 'verify_identity'].includes(value.toLowerCase())) }

export function parseDBeaverDataSources(contents: string): DBeaverMySQLConnection[] {
  let parsed: unknown
  try { parsed = JSON.parse(contents) } catch { throw new DBeaverProjectError('invalid data sources') }
  const rawConnectionData = asObject(parsed)?.connections
  const rawConnections = Array.isArray(rawConnectionData) ? rawConnectionData : Object.values(asObject(rawConnectionData) || {})
  const connections: DBeaverMySQLConnection[] = []
  for (const rawConnection of rawConnections) {
    const connection = asObject(rawConnection)
    const configuration = asObject(connection?.configuration)
    const url = parseMySQLURL(configuration?.url)
    if (!connection || (stringValue(connection.provider)?.toLowerCase() !== 'mysql' && !url)) continue
    const properties = asObject(configuration?.properties)
    const providerProperties = asObject(configuration?.['provider-properties'])
    const host = firstString([configuration], ['host']) || url?.host
    if (!host) continue
    connections.push({
      name: stringValue(connection.name) || host,
      host,
      port: portValue(configuration?.port) || url?.port || 3306,
      username: firstString([configuration, properties, providerProperties], ['user', 'username', 'userName']) || url?.username || '',
      initialDatabase: firstString([configuration], ['databaseName', 'database']) || url?.database || '',
      sslEnabled: sslValue(firstString([configuration, properties, providerProperties], ['useSSL', 'ssl', 'sslMode'])),
    })
  }
  return connections
}

function findEOCD(view: DataView): number {
  for (let offset = view.byteLength - 22; offset >= Math.max(0, view.byteLength - 65557); offset--) if (view.getUint32(offset, true) === ZIP_EOCD) return offset
  throw new DBeaverProjectError('invalid zip archive')
}
function dataSourcesEntries(bytes: Uint8Array): ZipEntry[] {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
  const end = findEOCD(view)
  if (view.getUint16(end + 4, true) || view.getUint16(end + 6, true)) throw new DBeaverProjectError('multi-disk zip archive')
  const count = view.getUint16(end + 10, true)
  const directorySize = view.getUint32(end + 12, true)
  let offset = view.getUint32(end + 16, true)
  if (offset + directorySize > bytes.byteLength) throw new DBeaverProjectError('invalid central directory')
  const decoder = new TextDecoder()
  const entries: ZipEntry[] = []
  for (let index = 0; index < count; index++) {
    if (offset + 46 > bytes.byteLength || view.getUint32(offset, true) !== ZIP_CENTRAL_FILE) throw new DBeaverProjectError('invalid zip entry')
    const compressedSize = view.getUint32(offset + 20, true)
    const uncompressedSize = view.getUint32(offset + 24, true)
    const nameLength = view.getUint16(offset + 28, true)
    const extraLength = view.getUint16(offset + 30, true)
    const commentLength = view.getUint16(offset + 32, true)
    const nextOffset = offset + 46 + nameLength + extraLength + commentLength
    if (nextOffset > bytes.byteLength) throw new DBeaverProjectError('invalid zip entry name')
    const name = decoder.decode(bytes.subarray(offset + 46, offset + 46 + nameLength))
    if (name.endsWith('/.dbeaver/data-sources.json')) entries.push({ compression: view.getUint16(offset + 10, true), compressedSize, uncompressedSize, localOffset: view.getUint32(offset + 42, true) })
    offset = nextOffset
  }
  return entries
}
async function extractEntry(bytes: Uint8Array, entry: ZipEntry): Promise<string> {
  if (entry.uncompressedSize > MAX_DATA_SOURCES_BYTES) throw new DBeaverProjectError('data sources file is too large')
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
  if (entry.localOffset + 30 > bytes.byteLength || view.getUint32(entry.localOffset, true) !== ZIP_LOCAL_FILE) throw new DBeaverProjectError('invalid local zip entry')
  const dataOffset = entry.localOffset + 30 + view.getUint16(entry.localOffset + 26, true) + view.getUint16(entry.localOffset + 28, true)
  const dataEnd = dataOffset + entry.compressedSize
  if (dataEnd > bytes.byteLength) throw new DBeaverProjectError('truncated zip entry')
  const compressed = bytes.subarray(dataOffset, dataEnd)
  let contents: Uint8Array
  if (entry.compression === 0) contents = compressed
  else if (entry.compression === 8 && typeof DecompressionStream !== 'undefined') {
    const copy = new Uint8Array(compressed.byteLength); copy.set(compressed)
    contents = new Uint8Array(await new Response(new Blob([copy.buffer]).stream().pipeThrough(new DecompressionStream('deflate-raw'))).arrayBuffer())
  } else throw new DBeaverProjectError('unsupported zip compression')
  if (contents.byteLength !== entry.uncompressedSize || contents.byteLength > MAX_DATA_SOURCES_BYTES) throw new DBeaverProjectError('invalid data sources file')
  return new TextDecoder().decode(contents)
}

export async function parseDBeaverProject(file: File): Promise<DBeaverMySQLConnection[]> {
  if (file.size > MAX_ARCHIVE_BYTES) throw new DBeaverProjectError('archive is too large')
  const bytes = new Uint8Array(await file.arrayBuffer())
  const entries = dataSourcesEntries(bytes)
  if (!entries.length) throw new DBeaverProjectError('data sources file not found')
  const parsed = (await Promise.all(entries.map(async (entry) => parseDBeaverDataSources(await extractEntry(bytes, entry))))).flat()
  const unique = new Map<string, DBeaverMySQLConnection>()
  for (const connection of parsed) unique.set(`${connection.name}\u0000${connection.host}\u0000${connection.port}`, connection)
  if (!unique.size) throw new DBeaverNoMySQLConnectionsError('no mysql connections found')
  return [...unique.values()]
}
