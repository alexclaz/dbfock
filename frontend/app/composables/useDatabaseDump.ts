// The dump is streamed, so the server cannot report a late failure through the
// status code. Both markers below are part of that contract: a dump is only
// complete when it ends with the footer the backend writes last.
const dumpFooter = 'SET FOREIGN_KEY_CHECKS=1;'
const dumpFailure = /^-- dump failed: (.*)$/m

export function useDatabaseDump() {
  const api = useApi()
  const { t } = useI18n()

  /** Fetches a whole database dump in a single request. */
  async function fetchDump(connectionId: string, database: string, structureOnly = false) {
    const query = structureOnly ? '?structureOnly=true' : ''
    const script = await api<string>(`/connections/${connectionId}/databases/${encodeURIComponent(database)}/dump${query}`, { responseType: 'text' })
    const failure = script.match(dumpFailure)
    if (failure) throw new Error(failure[1])
    if (!script.trimEnd().endsWith(dumpFooter)) throw new Error(t('database.tools.exportIncomplete', { database }))
    return script
  }

  function dumpFileName(database: string) {
    return `dbfock-dump-${database}-${new Date().toISOString().slice(0, 10)}.sql`.replaceAll(/[^a-zA-Z0-9._-]/g, '-')
  }

  return { fetchDump, dumpFileName }
}
