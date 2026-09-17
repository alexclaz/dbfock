package models

import "time"

type User struct {
	ID, Name, Email, PasswordHash string
	CreatedAt, UpdatedAt          time.Time
}
type Connection struct {
	ID, UserID, Name, Driver, Host, Username, InitialDatabase, Color, Environment string
	Port                                                                          int
	PasswordEncrypted                                                             string `json:"-"`
	SSLEnabled                                                                    bool
	TimeoutSeconds                                                                int
	CreatedAt, UpdatedAt                                                          time.Time
}
type ConnectionResponse struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Driver          string    `json:"driver"`
	Host            string    `json:"host"`
	Username        string    `json:"username"`
	InitialDatabase string    `json:"initialDatabase"`
	Color           string    `json:"color"`
	Environment     string    `json:"environment"`
	Port            int       `json:"port"`
	SSLEnabled      bool      `json:"sslEnabled"`
	TimeoutSeconds  int       `json:"timeoutSeconds"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func (c Connection) Public(status string) ConnectionResponse {
	return ConnectionResponse{ID: c.ID, Name: c.Name, Driver: c.Driver, Host: c.Host, Port: c.Port, Username: c.Username, InitialDatabase: c.InitialDatabase, Color: c.Color, Environment: c.Environment, SSLEnabled: c.SSLEnabled, TimeoutSeconds: c.TimeoutSeconds, Status: status, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
}

type TransactionStatus struct {
	Pending           bool                          `json:"pending"`
	PendingStatements int                           `json:"pendingStatements"`
	Statements        []PendingTransactionStatement `json:"statements"`
}

// PendingTransactionStatement is a mutation staged in the production transaction.
// Its ID is valid only while the transaction remains pending.
type PendingTransactionStatement struct {
	ID  string `json:"id"`
	SQL string `json:"sql"`
}

type QueryHistory struct {
	ID              string    `json:"id"`
	UserID          string    `json:"userId"`
	ConnectionID    string    `json:"connectionId"`
	SQL             string    `json:"sql"`
	Type            string    `json:"type"`
	Status          string    `json:"status"`
	ErrorMessage    string    `json:"errorMessage"`
	ExecutionTimeMs int64     `json:"executionTimeMs"`
	AffectedRows    int64     `json:"affectedRows"`
	CreatedAt       time.Time `json:"createdAt"`
}
type SavedQuery struct {
	ID           string    `json:"id"`
	UserID       string    `json:"-"`
	ConnectionID string    `json:"connectionId"`
	Name         string    `json:"name"`
	SQL          string    `json:"sql"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
type WorkspaceSmartQuery struct {
	ID           string            `json:"id"`
	UserID       string            `json:"-"`
	ConnectionID string            `json:"connectionId"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	SQL          string            `json:"sql"`
	SourceSQL    string            `json:"sourceSql,omitempty"`
	Parameters   []SmartQueryParam `json:"parameters"`
	CreatedAt    time.Time         `json:"createdAt"`
}
type QueryDayStat struct {
	Date    string `json:"date"`
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
}
type QueryOperationStat struct {
	Operation string `json:"operation"`
	Count     int    `json:"count"`
}
type SchemaStats struct {
	Available bool   `json:"available"`
	Databases int    `json:"databases"`
	Tables    int    `json:"tables"`
	Error     string `json:"error,omitempty"`
}
type ConnectionStats struct {
	SavedConnectionCount  int                  `json:"savedConnectionCount"`
	ActiveConnectionCount int                  `json:"activeConnectionCount"`
	TotalQueries          int                  `json:"totalQueries"`
	SuccessfulQueries     int                  `json:"successfulQueries"`
	FailedQueries         int                  `json:"failedQueries"`
	AverageExecutionMs    int64                `json:"averageExecutionMs"`
	AffectedRows          int64                `json:"affectedRows"`
	LastQueryAt           *time.Time           `json:"lastQueryAt,omitempty"`
	QueriesByDay          []QueryDayStat       `json:"queriesByDay"`
	QueriesByOperation    []QueryOperationStat `json:"queriesByOperation"`
	Schema                SchemaStats          `json:"schema"`
}

// MetadataTable is a tabular result for database connection metadata, such as
// server status, variables, storage engines, and granted privileges.
type MetadataTable struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}
type DatabaseInfo struct {
	Name string `json:"name"`
}

// DatabaseUser is a MySQL account available to the connection. An account is
// identified by both its username and allowed host.
type DatabaseUser struct {
	Username        string   `json:"username"`
	Host            string   `json:"host"`
	AuthPlugin      string   `json:"authPlugin"`
	Locked          bool     `json:"locked"`
	PasswordExpired bool     `json:"passwordExpired"`
	Grants          []string `json:"grants"`
}

// DatabaseUserInput contains the mutable details of a MySQL account. The
// database field scopes its privileges; an empty value means every database.
type DatabaseUserInput struct {
	Username   string   `json:"username"`
	Host       string   `json:"host"`
	Password   string   `json:"password,omitempty"`
	Privileges []string `json:"privileges"`
	Databases  []string `json:"databases"`
	// Database is retained while older clients transition to databases.
	Database string `json:"database,omitempty"`
}
type TableInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	ColumnCount int    `json:"columnCount"`
}

// SchemaComparison describes structural differences between two connections.
// Equal tables are counted in the summary but omitted from Databases to keep
// the response small even when large installations are nearly identical.
type SchemaComparison struct {
	SourceConnectionID string                     `json:"sourceConnectionId"`
	TargetConnectionID string                     `json:"targetConnectionId"`
	Summary            SchemaComparisonSummary    `json:"summary"`
	Databases          []DatabaseSchemaDifference `json:"databases"`
}
type SchemaComparisonSummary struct {
	DatabasesCompared int `json:"databasesCompared"`
	TablesCompared    int `json:"tablesCompared"`
	EqualTables       int `json:"equalTables"`
	DifferentTables   int `json:"differentTables"`
}
type DatabaseSchemaDifference struct {
	Name   string                  `json:"name"`
	Status string                  `json:"status"`
	Tables []TableSchemaDifference `json:"tables"`
}
type TableSchemaDifference struct {
	Name    string                   `json:"name"`
	Status  string                   `json:"status"`
	Columns []ColumnSchemaDifference `json:"columns"`
}
type ColumnSchemaDifference struct {
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	SourceType    string  `json:"sourceType,omitempty"`
	TargetType    string  `json:"targetType,omitempty"`
	SourceNull    *bool   `json:"sourceNullable,omitempty"`
	TargetNull    *bool   `json:"targetNullable,omitempty"`
	SourceDefault *string `json:"sourceDefault,omitempty"`
	TargetDefault *string `json:"targetDefault,omitempty"`
	SourceExtra   string  `json:"sourceExtra,omitempty"`
	TargetExtra   string  `json:"targetExtra,omitempty"`
	SourceKey     string  `json:"sourceKey,omitempty"`
	TargetKey     string  `json:"targetKey,omitempty"`
}

type DatabaseMigrationOptions struct {
	Databases         []string                     `json:"databases"`
	MaxTableSizeBytes int64                        `json:"maxTableSizeBytes"`
	Strategy          string                       `json:"strategy"`
	TargetDatabase    string                       `json:"targetDatabase,omitempty"`
	RecreateTarget    bool                         `json:"recreateTarget,omitempty"`
	StructureOnly     bool                         `json:"structureOnly,omitempty"`
	IgnoreDuplicates  bool                         `json:"ignoreDuplicates,omitempty"`
	CreateMissing     bool                         `json:"createMissingTables,omitempty"`
	SkipMatching      bool                         `json:"skipMatchingTables,omitempty"`
	SelectedTables    []DatabaseMigrationSelection `json:"selectedTables,omitempty"`
}
type DatabaseMigrationSelection struct {
	Database string `json:"database"`
	Table    string `json:"table"`
}
type DatabaseMigrationPlan struct {
	Databases      int                      `json:"databases"`
	Tables         []DatabaseMigrationTable `json:"tables"`
	SkippedTables  []DatabaseMigrationSkip  `json:"skippedTables"`
	EstimatedRows  int64                    `json:"estimatedRows"`
	TotalSizeBytes int64                    `json:"totalSizeBytes"`
}
type DatabaseMigrationTable struct {
	Database        string `json:"database"`
	Table           string `json:"table"`
	SizeBytes       int64  `json:"sizeBytes"`
	TargetSizeBytes *int64 `json:"targetSizeBytes,omitempty"`
	EstimatedRows   int64  `json:"estimatedRows"`
}
type DatabaseMigrationResult struct {
	DatabasesMigrated int                        `json:"databasesMigrated"`
	TablesMigrated    int                        `json:"tablesMigrated"`
	RowsMigrated      int64                      `json:"rowsMigrated"`
	SkippedTables     []DatabaseMigrationSkip    `json:"skippedTables"`
	FailedTables      []DatabaseMigrationFailure `json:"failedTables"`
	ExecutionTimeMs   int64                      `json:"executionTimeMs"`
}
type DatabaseMigrationFailure struct {
	Database string `json:"database"`
	Table    string `json:"table"`
	Error    string `json:"error"`
}
type DatabaseMigrationProgress struct {
	TotalTables               int    `json:"totalTables"`
	CompletedTables           int    `json:"completedTables"`
	CurrentDatabase           string `json:"currentDatabase,omitempty"`
	CurrentTable              string `json:"currentTable,omitempty"`
	CurrentTableRows          int64  `json:"currentTableRows"`
	CurrentTableEstimatedRows int64  `json:"currentTableEstimatedRows"`
	RowsMigrated              int64  `json:"rowsMigrated"`
}
type DatabaseMigrationJob struct {
	ID                 string                    `json:"id"`
	SourceConnectionID string                    `json:"sourceConnectionId"`
	TargetConnectionID string                    `json:"targetConnectionId"`
	Status             string                    `json:"status"`
	Progress           DatabaseMigrationProgress `json:"progress"`
	Result             *DatabaseMigrationResult  `json:"result,omitempty"`
	Error              string                    `json:"error,omitempty"`
	CreatedAt          time.Time                 `json:"createdAt"`
	UpdatedAt          time.Time                 `json:"updatedAt"`
}
type DatabaseMigrationSkip struct {
	Database  string `json:"database"`
	Table     string `json:"table"`
	SizeBytes int64  `json:"sizeBytes"`
	Reason    string `json:"reason"`
}
type ColumnInfo struct {
	Name         string  `json:"name"`
	DatabaseType string  `json:"databaseType"`
	ColumnType   string  `json:"columnType"`
	Nullable     bool    `json:"nullable"`
	Key          *string `json:"key"`
	Default      *string `json:"default"`
	Extra        string  `json:"extra"`
}
type IndexInfo struct {
	Name    string   `json:"name"`
	Unique  bool     `json:"unique"`
	Columns []string `json:"columns"`
}
type ForeignKeyInfo struct {
	Name             string `json:"name"`
	Column           string `json:"column"`
	ReferencedTable  string `json:"referencedTable"`
	ReferencedColumn string `json:"referencedColumn"`
}
type ConstraintInfo struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Columns []string `json:"columns"`
}
type ReferenceInfo struct {
	Name             string `json:"name"`
	Database         string `json:"database"`
	Table            string `json:"table"`
	Column           string `json:"column"`
	ReferencedColumn string `json:"referencedColumn"`
}
type TriggerInfo struct {
	Name      string `json:"name"`
	Timing    string `json:"timing"`
	Event     string `json:"event"`
	Statement string `json:"statement"`
}
type TableStructure struct {
	Columns     []ColumnInfo     `json:"columns"`
	Constraints []ConstraintInfo `json:"constraints"`
	Indexes     []IndexInfo      `json:"indexes"`
	ForeignKeys []ForeignKeyInfo `json:"foreignKeys"`
	References  []ReferenceInfo  `json:"references"`
	Triggers    []TriggerInfo    `json:"triggers"`
	DDL         string           `json:"ddl"`
}
type DiagramTable struct {
	Name        string           `json:"name"`
	Columns     []ColumnInfo     `json:"columns"`
	ForeignKeys []ForeignKeyInfo `json:"foreignKeys"`
}
type SchemaDiagram struct {
	Tables []DiagramTable `json:"tables"`
}
type QueryColumn struct {
	Name         string `json:"name"`
	DatabaseType string `json:"databaseType"`
	Nullable     bool   `json:"nullable"`
}
type QueryResult struct {
	Columns            []QueryColumn    `json:"columns"`
	Rows               []map[string]any `json:"rows"`
	RowCount           int              `json:"rowCount"`
	ExecutionTimeMs    int64            `json:"executionTimeMs"`
	AffectedRows       int64            `json:"affectedRows"`
	LastInsertID       int64            `json:"lastInsertId,omitempty"`
	HasMore            bool             `json:"hasMore"`
	Warnings           []string         `json:"warnings,omitempty"`
	TransactionPending bool             `json:"transactionPending"`
	PendingStatements  int              `json:"pendingStatements"`
}
type AISetting struct {
	UserID, Provider, Model, BaseURL, APIKeyEncrypted string
	UpdatedAt                                         time.Time
}
type AISettingResponse struct {
	Configured bool      `json:"configured"`
	Provider   string    `json:"provider"`
	Model      string    `json:"model"`
	BaseURL    string    `json:"baseUrl"`
	HasAPIKey  bool      `json:"hasApiKey"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type BackupSetting struct {
	UserID, Endpoint, Bucket, Region, AccessKeyEncrypted, SecretEncrypted string
	UpdatedAt                                                             time.Time
}

type BackupSettingResponse struct {
	Configured   bool      `json:"configured"`
	Endpoint     string    `json:"endpoint"`
	Bucket       string    `json:"bucket"`
	Region       string    `json:"region"`
	HasAccessKey bool      `json:"hasAccessKey"`
	HasSecret    bool      `json:"hasSecret"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Backup struct {
	Key       string    `json:"key"`
	CreatedAt time.Time `json:"createdAt"`
	Size      int64     `json:"size"`
}

type AIAuditLog struct {
	ID        string    `json:"id"`
	RunID     string    `json:"runId"`
	Question  string    `json:"question"`
	Stage     string    `json:"stage"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Request   string    `json:"request"`
	Response  string    `json:"response"`
	Error     string    `json:"error"`
	CreatedAt time.Time `json:"createdAt"`
}

type AIChatJob struct {
	ID             string    `json:"id"`
	Status         string    `json:"status"`
	Message        string    `json:"message,omitempty"`
	PartialMessage string    `json:"partialMessage,omitempty"`
	Error          string    `json:"error,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// SmartQuery is an AI-curated, read-only query template kept in the client
// workspace. It deliberately has no database type metadata.
type SmartQuery struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	SQL         string            `json:"sql"`
	Parameters  []SmartQueryParam `json:"parameters"`
}

type SmartQueryParam struct {
	Key          string `json:"key"`
	DefaultValue string `json:"defaultValue"`
}

func (s AISetting) Public() AISettingResponse {
	return AISettingResponse{Configured: true, Provider: s.Provider, Model: s.Model, BaseURL: s.BaseURL, HasAPIKey: s.APIKeyEncrypted != "", UpdatedAt: s.UpdatedAt}
}

func (s BackupSetting) Public() BackupSettingResponse {
	return BackupSettingResponse{Configured: true, Endpoint: s.Endpoint, Bucket: s.Bucket, Region: s.Region, HasAccessKey: s.AccessKeyEncrypted != "", HasSecret: s.SecretEncrypted != "", UpdatedAt: s.UpdatedAt}
}
