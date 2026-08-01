package database

import (
	"context"
	"fmt"
	"regexp"

	"github.com/dbfock/database-manager/backend/internal/models"
)

var validIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]*$`)

func ValidateIdentifier(identifier string) error {
	if !validIdentifier.MatchString(identifier) {
		return fmt.Errorf("invalid database identifier")
	}
	return nil
}
func QuoteIdentifier(identifier string) (string, error) {
	if err := ValidateIdentifier(identifier); err != nil {
		return "", err
	}
	return "`" + identifier + "`", nil
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

// TransactionalProvider is implemented by drivers that can keep a manual
// transaction open between query requests.
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
