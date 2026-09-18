package database

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/dbfock/database-manager/backend/internal/models"
)

func ValidateIdentifier(identifier string) error {
	// MySQL permits quoted identifiers to contain spaces, punctuation, Unicode,
	// and to start with a digit. Keep validation limited to constraints shared
	// by database, table, and column names; QuoteIdentifier handles SQL safety.
	if identifier == "" || !utf8.ValidString(identifier) || strings.ContainsRune(identifier, 0) || utf8.RuneCountInString(identifier) > 64 {
		return fmt.Errorf("invalid SQL identifier")
	}
	return nil
}
func QuoteIdentifier(identifier string) (string, error) {
	if err := ValidateIdentifier(identifier); err != nil {
		return "", err
	}
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`", nil
}

type Provider interface {
	TestConnection(context.Context, models.Connection) error
	ListDatabases(context.Context, models.Connection) ([]models.DatabaseInfo, error)
	ListTables(context.Context, models.Connection, string, bool) ([]models.TableInfo, error)
	GetTableStructure(context.Context, models.Connection, string, string) (*models.TableStructure, error)
	GetSchemaDiagram(context.Context, models.Connection, string) (*models.SchemaDiagram, error)
	GetTableData(context.Context, models.Connection, string, string, int, int, string, string) (*models.QueryResult, error)
	ConnectionMetadata(context.Context, models.Connection, string) (models.MetadataTable, error)
	Query(context.Context, models.Connection, string, int) (*models.QueryResult, error)
}

// TransactionalProvider is implemented by drivers that can stage production
// mutations and execute only the approved statements on commit.
type TransactionalProvider interface {
	QueryInTransaction(context.Context, models.Connection, string, int, bool) (*models.QueryResult, error)
	CommitTransaction(context.Context, models.Connection, []string) (models.TransactionStatus, error)
	RollbackTransaction(context.Context, models.Connection, []string) (models.TransactionStatus, error)
	TransactionStatus(models.Connection) models.TransactionStatus
}

// RowUpdater is implemented by drivers that can safely update a row using its
// original values as the optimistic-concurrency predicate.
type RowUpdater interface {
	UpdateRow(context.Context, models.Connection, string, string, map[string]any, map[string]any) (*models.QueryResult, error)
	UpdateRowInTransaction(context.Context, models.Connection, string, string, map[string]any, map[string]any) (*models.QueryResult, error)
}

// RowModifier is implemented by drivers that can insert and delete result-grid
// rows with parameterized statements.
type RowModifier interface {
	InsertRow(context.Context, models.Connection, string, string, map[string]any) (*models.QueryResult, error)
	InsertRowInTransaction(context.Context, models.Connection, string, string, map[string]any) (*models.QueryResult, error)
	DeleteRow(context.Context, models.Connection, string, string, map[string]any) (*models.QueryResult, error)
	DeleteRowInTransaction(context.Context, models.Connection, string, string, map[string]any) (*models.QueryResult, error)
}

// UserManager is implemented by providers that support native database-account
// management. It is deliberately optional because not every driver exposes it.
type UserManager interface {
	ListUsers(context.Context, models.Connection) ([]models.DatabaseUser, error)
	CreateUser(context.Context, models.Connection, models.DatabaseUserInput) error
	UpdateUser(context.Context, models.Connection, string, string, models.DatabaseUserInput) error
	DeleteUser(context.Context, models.Connection, string, string) error
}

// DatabaseDumpRestorer restores a SQL dump into one named database. The
// implementation is responsible for recreating the target first, so callers
// never need to issue a destructive drop as a separate connection request.
type DatabaseDumpRestorer interface {
	RestoreDatabase(context.Context, models.Connection, string, string) (*models.QueryResult, error)
}

// DatabaseDumper writes a SQL dump of one database. It streams, so a dump is a
// single request whose memory cost does not grow with the data volume.
type DatabaseDumper interface {
	DumpDatabase(ctx context.Context, c models.Connection, databaseName string, structureOnly bool, w io.Writer) error
}

// DatabaseRecreator drops a database and creates it empty again, keeping its
// character set and collation. Dropping the whole schema is the only way to
// clear tables that reference each other, because individual drops fail on the
// foreign keys pointing at them.
type DatabaseRecreator interface {
	RecreateDatabase(context.Context, models.Connection, string) (*models.QueryResult, error)
}

// ConnectionTooler performs server-to-server operations without routing table
// contents through the browser. Implementations must stream/batch row copying
// so memory use stays bounded for large databases.
type ConnectionTooler interface {
	CompareSchemas(context.Context, models.Connection, models.Connection) (*models.SchemaComparison, error)
	PlanMigration(context.Context, models.Connection, models.Connection, models.DatabaseMigrationOptions) (*models.DatabaseMigrationPlan, error)
	MigrateDatabases(context.Context, models.Connection, models.Connection, models.DatabaseMigrationOptions, func(models.DatabaseMigrationProgress)) (*models.DatabaseMigrationResult, error)
}

type Registry struct{ providers map[string]Provider }

func NewRegistry() *Registry                                  { return &Registry{providers: map[string]Provider{}} }
func (r *Registry) Register(driver string, provider Provider) { r.providers[driver] = provider }
func (r *Registry) Get(driver string) (Provider, error) {
	p, ok := r.providers[driver]
	if !ok {
		return nil, fmt.Errorf("unsupported database driver: %s", driver)
	}
	return p, nil
}
