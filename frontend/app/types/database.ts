export interface Connection { id: string; name: string; driver: string; host: string; port: number; username: string; initialDatabase: string; color: string; environment: 'development' | 'production'; sslEnabled: boolean; timeoutSeconds: number; status: 'connected' | 'disconnected'; createdAt: string }
export interface ConnectionInput { name: string; driver: 'mysql'; host: string; port: number; username: string; password?: string; initialDatabase?: string; color: string; environment: 'development' | 'production'; sslEnabled: boolean; timeoutSeconds: number }
export interface DatabaseInfo { name: string }
export interface TableInfo { name: string; type: string }
export interface ColumnInfo { name: string; databaseType: string; columnType: string; nullable: boolean; key: string | null; default: string | null; extra: string }
export interface TableStructure { columns: ColumnInfo[]; constraints: { name: string; type: string; columns: string[] }[]; indexes: { name: string; unique: boolean; columns: string[] }[]; foreignKeys: { name: string; column: string; referencedTable: string; referencedColumn: string }[]; references: { name: string; database: string; table: string; column: string; referencedColumn: string }[]; triggers: { name: string; timing: string; event: string; statement: string }[]; ddl: string }
export interface QueryColumn { name: string; databaseType: string; nullable: boolean }
export interface QueryResult { columns: QueryColumn[]; rows: Record<string, unknown>[]; rowCount: number; executionTimeMs: number; affectedRows: number; lastInsertId?: number; hasMore: boolean; transactionPending: boolean; pendingStatements: number }
export interface TransactionStatus { pending: boolean; pendingStatements: number }
export interface QueryHistory { id: string; connectionId: string; sql: string; type: string; status: 'success' | 'error'; errorMessage: string; executionTimeMs: number; affectedRows: number; createdAt: string }
export interface ConnectionStats { savedConnectionCount: number; activeConnectionCount: number; totalQueries: number; successfulQueries: number; failedQueries: number; averageExecutionMs: number; affectedRows: number; lastQueryAt?: string; queriesByDay: { date: string; success: number; failed: number }[]; queriesByOperation: { operation: string; count: number }[]; schema: { available: boolean; databases: number; tables: number; error?: string } }
export interface ConnectionMetadata { columns: string[]; rows: string[][] }
export interface SavedQuery { id: string; name: string; connectionId: string; sql: string; updatedAt: string }
export interface SmartQueryParameter { key: string; defaultValue: string }
export interface SmartQuery { id: string; connectionId: string; title: string; description: string; sql: string; sourceSql?: string; parameters: SmartQueryParameter[]; createdAt: string }
export interface AIAgentMessage { role: 'user' | 'assistant'; content: string; executionTimeMs?: number }
export interface AISchemaTable { database: string; table: string }
export interface AIAgentChat { draft: string; messages: AIAgentMessage[]; includeEditorQuery?: boolean; databaseScope?: 'all' | 'selected'; selectedDatabases?: string[]; tableScope?: 'all' | 'selected'; selectedTables?: AISchemaTable[] }
export interface AIChatJob { id: string; status: 'running' | 'complete' | 'failed'; message?: string; error?: string; createdAt: string; updatedAt: string }
export interface WorkspaceTab { id: string; title: string; type: 'empty' | 'welcome' | 'saved' | 'smart' | 'sql' | 'table' | 'settings' | 'stats'; connectionId?: string; executionConnectionId?: string; database?: string; table?: string; sql?: string; tableSection?: 'data' | 'structure' | 'constraints' | 'foreignKeys' | 'references' | 'triggers' | 'indexes' | 'query' | 'ddl'; settingsSection?: 'appearance' | 'shortcuts' | 'connections' | 'ai' | 'audit'; aiChat?: AIAgentChat; aiJobId?: string; aiStatus?: 'running' | 'complete'; dirty?: boolean; savedQueryId?: string }
