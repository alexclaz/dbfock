package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dbfock/database-manager/backend/internal/database"
	"github.com/dbfock/database-manager/backend/internal/models"
	driver "github.com/go-sql-driver/mysql"
)

type transactionSession struct {
	mu         sync.Mutex
	statements []pendingStatement
	nextID     int
}

type pendingStatement struct {
	models.PendingTransactionStatement
	args []any
}

type Provider struct {
	maxOpen       int
	transactions  map[string]*transactionSession
	transactionMu sync.Mutex
}

func New(maxOpen int) *Provider {
	return &Provider{maxOpen: maxOpen, transactions: map[string]*transactionSession{}}
}
func (p *Provider) open(c models.Connection) (*sql.DB, error) {
	cfg := driver.NewConfig()
	cfg.User = c.Username
	cfg.Passwd = c.PasswordEncrypted
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", c.Host, c.Port)
	cfg.DBName = c.InitialDatabase
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.AllowNativePasswords = true
	cfg.MultiStatements = true
	// MySQL does not accept server-side parameter markers in CREATE/ALTER USER
	// authentication clauses. The driver safely expands those values before the
	// request is sent, so account passwords remain parameterized at this layer.
	cfg.InterpolateParams = true
	if c.SSLEnabled {
		cfg.TLSConfig = "preferred"
	}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(p.maxOpen)
	db.SetMaxIdleConns(min(2, p.maxOpen))
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func (p *Provider) withDB(c models.Connection, fn func(*sql.DB) error) error {
	db, err := p.open(c)
	if err != nil {
		return err
	}
	defer db.Close()
	return fn(db)
}
func (p *Provider) TestConnection(ctx context.Context, c models.Connection) error {
	return p.withDB(c, func(db *sql.DB) error { return db.PingContext(ctx) })
}
func (p *Provider) ListDatabases(ctx context.Context, c models.Connection) (out []models.DatabaseInfo, err error) {
	out = make([]models.DatabaseInfo, 0)
	err = p.withDB(c, func(db *sql.DB) error {
		rows, e := db.QueryContext(ctx, "SHOW DATABASES")
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var n string
			if e = rows.Scan(&n); e != nil {
				return e
			}
			out = append(out, models.DatabaseInfo{Name: n})
		}
		return rows.Err()
	})
	return
}

var supportedUserPrivileges = map[string]bool{
	"ALL PRIVILEGES": true,
	"SELECT":         true, "INSERT": true, "UPDATE": true, "DELETE": true,
	"CREATE": true, "ALTER": true, "DROP": true, "INDEX": true,
	"REFERENCES": true, "EXECUTE": true, "CREATE VIEW": true,
	"SHOW VIEW": true, "TRIGGER": true,
}

var validMySQLUsername = regexp.MustCompile(`^[A-Za-z0-9_.@-]+$`)
var validMySQLHost = regexp.MustCompile(`^[A-Za-z0-9.%_:-]+$`)

func mysqlAccount(username, host string) (string, error) {
	if !validMySQLUsername.MatchString(username) || len(username) > 32 {
		return "", fmt.Errorf("username must contain between 1 and 32 characters")
	}
	if !validMySQLHost.MatchString(host) || len(host) > 255 {
		return "", fmt.Errorf("host must contain between 1 and 255 characters")
	}
	return "'" + username + "'@'" + host + "'", nil
}

func userPrivileges(input models.DatabaseUserInput) ([]string, error) {
	if len(input.Privileges) == 0 {
		return nil, fmt.Errorf("select at least one privilege")
	}
	seen := map[string]bool{}
	privileges := make([]string, 0, len(input.Privileges))
	for _, privilege := range input.Privileges {
		privilege = strings.ToUpper(strings.TrimSpace(privilege))
		if !supportedUserPrivileges[privilege] {
			return nil, fmt.Errorf("unsupported privilege: %s", privilege)
		}
		if !seen[privilege] {
			seen[privilege] = true
			privileges = append(privileges, privilege)
		}
	}
	if seen["ALL PRIVILEGES"] {
		if len(privileges) != 1 {
			return nil, fmt.Errorf("ALL PRIVILEGES cannot be combined with individual privileges")
		}
		return privileges, nil
	}
	if len(privileges) == len(supportedUserPrivileges)-1 {
		return []string{"ALL PRIVILEGES"}, nil
	}
	sort.Strings(privileges)
	return privileges, nil
}

func privilegeScopes(input models.DatabaseUserInput) ([]string, error) {
	databases := input.Databases
	if len(databases) == 0 && strings.TrimSpace(input.Database) != "" {
		databases = []string{input.Database}
	}
	if len(databases) == 0 {
		return []string{"*.*"}, nil
	}
	seen := map[string]bool{}
	scopes := make([]string, 0, len(databases))
	for _, databaseName := range databases {
		if seen[databaseName] {
			continue
		}
		quoted, err := database.QuoteIdentifier(databaseName)
		if err != nil {
			return nil, err
		}
		seen[databaseName] = true
		scopes = append(scopes, quoted+".*")
	}
	if len(scopes) == 0 {
		return []string{"*.*"}, nil
	}
	return scopes, nil
}

func grantStatements(account string, input models.DatabaseUserInput) ([]string, error) {
	privileges, err := userPrivileges(input)
	if err != nil {
		return nil, err
	}
	scopes, err := privilegeScopes(input)
	if err != nil {
		return nil, err
	}
	statements := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		statements = append(statements, "GRANT "+strings.Join(privileges, ", ")+" ON "+scope+" TO "+account)
	}
	return statements, nil
}

func (p *Provider) ListUsers(ctx context.Context, c models.Connection) (out []models.DatabaseUser, err error) {
	out = make([]models.DatabaseUser, 0)
	err = p.withDB(c, func(db *sql.DB) error {
		rows, queryErr := db.QueryContext(ctx, "SELECT user, host, plugin, account_locked, password_expired FROM mysql.user ORDER BY user, host")
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		for rows.Next() {
			var item models.DatabaseUser
			var locked, expired string
			if scanErr := rows.Scan(&item.Username, &item.Host, &item.AuthPlugin, &locked, &expired); scanErr != nil {
				return scanErr
			}
			item.Locked = locked == "Y"
			item.PasswordExpired = expired == "Y"
			out = append(out, item)
		}
		if queryErr = rows.Err(); queryErr != nil {
			return queryErr
		}
		rows.Close()
		for i := range out {
			account, accountErr := mysqlAccount(out[i].Username, out[i].Host)
			if accountErr != nil {
				return accountErr
			}
			grants, grantErr := db.QueryContext(ctx, "SHOW GRANTS FOR "+account)
			if grantErr != nil {
				return grantErr
			}
			for grants.Next() {
				var grant string
				if scanErr := grants.Scan(&grant); scanErr != nil {
					grants.Close()
					return scanErr
				}
				out[i].Grants = append(out[i].Grants, grant)
			}
			if grantErr = grants.Err(); grantErr != nil {
				grants.Close()
				return grantErr
			}
			grants.Close()
		}
		return nil
	})
	return
}

func (p *Provider) CreateUser(ctx context.Context, c models.Connection, input models.DatabaseUserInput) error {
	if input.Password == "" {
		return fmt.Errorf("password is required")
	}
	account, err := mysqlAccount(input.Username, input.Host)
	if err != nil {
		return err
	}
	grants, err := grantStatements(account, input)
	if err != nil {
		return err
	}
	return p.withDB(c, func(db *sql.DB) error {
		if _, execErr := db.ExecContext(ctx, "CREATE USER "+account+" IDENTIFIED BY ?", input.Password); execErr != nil {
			return execErr
		}
		for _, grant := range grants {
			if _, execErr := db.ExecContext(ctx, grant); execErr != nil {
				_, _ = db.ExecContext(context.Background(), "DROP USER "+account)
				return execErr
			}
		}
		return nil
	})
}

func (p *Provider) UpdateUser(ctx context.Context, c models.Connection, username, host string, input models.DatabaseUserInput) error {
	account, err := mysqlAccount(username, host)
	if err != nil {
		return err
	}
	grants, err := grantStatements(account, input)
	if err != nil {
		return err
	}
	return p.withDB(c, func(db *sql.DB) error {
		if input.Password != "" {
			if _, execErr := db.ExecContext(ctx, "ALTER USER "+account+" IDENTIFIED BY ?", input.Password); execErr != nil {
				return execErr
			}
		}
		if _, execErr := db.ExecContext(ctx, "REVOKE ALL PRIVILEGES, GRANT OPTION FROM "+account); execErr != nil {
			return execErr
		}
		for _, grant := range grants {
			if _, execErr := db.ExecContext(ctx, grant); execErr != nil {
				return execErr
			}
		}
		return nil
	})
}

func (p *Provider) DeleteUser(ctx context.Context, c models.Connection, username, host string) error {
	account, err := mysqlAccount(username, host)
	if err != nil {
		return err
	}
	return p.withDB(c, func(db *sql.DB) error {
		_, execErr := db.ExecContext(ctx, "DROP USER "+account)
		return execErr
	})
}
func (p *Provider) ListTables(ctx context.Context, c models.Connection, dbName string, views bool) (out []models.TableInfo, err error) {
	out = make([]models.TableInfo, 0)
	if err = database.ValidateIdentifier(dbName); err != nil {
		return
	}
	kind := "BASE TABLE"
	if views {
		kind = "VIEW"
	}
	err = p.withDB(c, func(db *sql.DB) error {
		rows, e := db.QueryContext(ctx, "SELECT t.table_name, t.table_type, COUNT(c.column_name) FROM information_schema.tables t LEFT JOIN information_schema.columns c ON c.table_schema = t.table_schema AND c.table_name = t.table_name WHERE t.table_schema = ? AND t.table_type = ? GROUP BY t.table_name, t.table_type ORDER BY t.table_name", dbName, kind)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var n, t string
			var columnCount int
			if e = rows.Scan(&n, &t, &columnCount); e != nil {
				return e
			}
			out = append(out, models.TableInfo{Name: n, Type: t, ColumnCount: columnCount})
		}
		return rows.Err()
	})
	return
}
func (p *Provider) ConnectionMetadata(ctx context.Context, c models.Connection, section string) (out models.MetadataTable, err error) {
	queries := map[string]string{
		"session-status":    "SHOW SESSION STATUS",
		"global-status":     "SHOW GLOBAL STATUS",
		"session-variables": "SHOW SESSION VARIABLES",
		"global-variables":  "SHOW GLOBAL VARIABLES",
		"engines":           "SHOW ENGINES",
		"user-privileges":   "SHOW GRANTS FOR CURRENT_USER()",
		"plugins":           "SHOW PLUGINS",
	}
	statement, ok := queries[section]
	if !ok {
		return out, fmt.Errorf("unsupported metadata section: %s", section)
	}
	out.Rows = make([][]string, 0)
	err = p.withDB(c, func(db *sql.DB) error {
		rows, queryErr := db.QueryContext(ctx, statement)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		columns, columnsErr := rows.Columns()
		if columnsErr != nil {
			return columnsErr
		}
		out.Columns = columns
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		for rows.Next() {
			if scanErr := rows.Scan(pointers...); scanErr != nil {
				return scanErr
			}
			row := make([]string, len(columns))
			for i, value := range values {
				switch typed := value.(type) {
				case nil:
					row[i] = ""
				case []byte:
					row[i] = string(typed)
				default:
					row[i] = fmt.Sprint(typed)
				}
			}
			out.Rows = append(out.Rows, row)
		}
		return rows.Err()
	})
	return out, err
}
func (p *Provider) GetTableStructure(ctx context.Context, c models.Connection, dbName, table string) (result *models.TableStructure, err error) {
	if err = database.ValidateIdentifier(dbName); err != nil {
		return
	}
	if err = database.ValidateIdentifier(table); err != nil {
		return
	}
	result = &models.TableStructure{
		Columns:     []models.ColumnInfo{},
		Constraints: []models.ConstraintInfo{},
		Indexes:     []models.IndexInfo{},
		ForeignKeys: []models.ForeignKeyInfo{},
		References:  []models.ReferenceInfo{},
		Triggers:    []models.TriggerInfo{},
	}
	err = p.withDB(c, func(db *sql.DB) error {
		rows, e := db.QueryContext(ctx, `SELECT column_name,data_type,column_type,is_nullable,column_key,column_default,extra FROM information_schema.columns WHERE table_schema=? AND table_name=? ORDER BY ordinal_position`, dbName, table)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var x models.ColumnInfo
			var nullable string
			var def sql.NullString
			if e = rows.Scan(&x.Name, &x.DatabaseType, &x.ColumnType, &nullable, &x.Key, &def, &x.Extra); e != nil {
				return e
			}
			x.Nullable = nullable == "YES"
			if def.Valid {
				x.Default = &def.String
			}
			result.Columns = append(result.Columns, x)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		constraints, e := db.QueryContext(ctx, `SELECT tc.constraint_name,tc.constraint_type,kcu.column_name FROM information_schema.table_constraints tc LEFT JOIN information_schema.key_column_usage kcu ON tc.constraint_schema=kcu.constraint_schema AND tc.table_name=kcu.table_name AND tc.constraint_name=kcu.constraint_name WHERE tc.table_schema=? AND tc.table_name=? ORDER BY tc.constraint_name,kcu.ordinal_position`, dbName, table)
		if e != nil {
			return e
		}
		defer constraints.Close()
		constraintByName := map[string]*models.ConstraintInfo{}
		constraintOrder := []string{}
		for constraints.Next() {
			var name, kind string
			var column sql.NullString
			if e = constraints.Scan(&name, &kind, &column); e != nil {
				return e
			}
			if constraintByName[name] == nil {
				constraintByName[name] = &models.ConstraintInfo{Name: name, Type: kind, Columns: []string{}}
				constraintOrder = append(constraintOrder, name)
			}
			if column.Valid {
				constraintByName[name].Columns = append(constraintByName[name].Columns, column.String)
			}
		}
		if e = constraints.Err(); e != nil {
			return e
		}
		for _, name := range constraintOrder {
			result.Constraints = append(result.Constraints, *constraintByName[name])
		}
		idx, e := db.QueryContext(ctx, `SELECT index_name,non_unique,column_name FROM information_schema.statistics WHERE table_schema=? AND table_name=? ORDER BY index_name,seq_in_index`, dbName, table)
		if e != nil {
			return e
		}
		defer idx.Close()
		indices := map[string]*models.IndexInfo{}
		order := []string{}
		for idx.Next() {
			var n, col string
			var non bool
			if e = idx.Scan(&n, &non, &col); e != nil {
				return e
			}
			if indices[n] == nil {
				indices[n] = &models.IndexInfo{Name: n, Unique: !non}
				order = append(order, n)
			}
			indices[n].Columns = append(indices[n].Columns, col)
		}
		for _, n := range order {
			result.Indexes = append(result.Indexes, *indices[n])
		}
		fq, e := db.QueryContext(ctx, `SELECT constraint_name,column_name,referenced_table_name,referenced_column_name FROM information_schema.key_column_usage WHERE table_schema=? AND table_name=? AND referenced_table_name IS NOT NULL`, dbName, table)
		if e != nil {
			return e
		}
		defer fq.Close()
		for fq.Next() {
			var f models.ForeignKeyInfo
			if e = fq.Scan(&f.Name, &f.Column, &f.ReferencedTable, &f.ReferencedColumn); e != nil {
				return e
			}
			result.ForeignKeys = append(result.ForeignKeys, f)
		}
		if e = fq.Err(); e != nil {
			return e
		}
		references, e := db.QueryContext(ctx, `SELECT constraint_name,table_schema,table_name,column_name,referenced_column_name FROM information_schema.key_column_usage WHERE referenced_table_schema=? AND referenced_table_name=? ORDER BY table_schema,table_name,constraint_name,ordinal_position`, dbName, table)
		if e != nil {
			return e
		}
		defer references.Close()
		for references.Next() {
			var reference models.ReferenceInfo
			if e = references.Scan(&reference.Name, &reference.Database, &reference.Table, &reference.Column, &reference.ReferencedColumn); e != nil {
				return e
			}
			result.References = append(result.References, reference)
		}
		if e = references.Err(); e != nil {
			return e
		}
		triggers, e := db.QueryContext(ctx, `SELECT trigger_name,action_timing,event_manipulation,action_statement FROM information_schema.triggers WHERE event_object_schema=? AND event_object_table=? ORDER BY trigger_name`, dbName, table)
		if e != nil {
			return e
		}
		defer triggers.Close()
		for triggers.Next() {
			var trigger models.TriggerInfo
			if e = triggers.Scan(&trigger.Name, &trigger.Timing, &trigger.Event, &trigger.Statement); e != nil {
				return e
			}
			result.Triggers = append(result.Triggers, trigger)
		}
		if e = triggers.Err(); e != nil {
			return e
		}
		qdb, _ := database.QuoteIdentifier(dbName)
		qt, _ := database.QuoteIdentifier(table)
		var name, ddl string
		if e = db.QueryRowContext(ctx, "SHOW CREATE TABLE "+qdb+"."+qt).Scan(&name, &ddl); e == nil {
			result.DDL = ddl
		}
		return nil
	})
	return
}
func (p *Provider) GetSchemaDiagram(ctx context.Context, c models.Connection, dbName string) (result *models.SchemaDiagram, err error) {
	if err = database.ValidateIdentifier(dbName); err != nil {
		return
	}
	result = &models.SchemaDiagram{Tables: []models.DiagramTable{}}
	err = p.withDB(c, func(db *sql.DB) error {
		byName := map[string]*models.DiagramTable{}
		order := []string{}
		tables, e := db.QueryContext(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema=? AND table_type='BASE TABLE' ORDER BY table_name`, dbName)
		if e != nil {
			return e
		}
		defer tables.Close()
		for tables.Next() {
			var name string
			if e = tables.Scan(&name); e != nil {
				return e
			}
			byName[name] = &models.DiagramTable{Name: name, Columns: []models.ColumnInfo{}, ForeignKeys: []models.ForeignKeyInfo{}}
			order = append(order, name)
		}
		if e = tables.Err(); e != nil {
			return e
		}
		defer func() {
			for _, name := range order {
				result.Tables = append(result.Tables, *byName[name])
			}
		}()
		columns, e := db.QueryContext(ctx, `SELECT table_name,column_name,data_type,column_type,is_nullable,column_key,column_default,extra FROM information_schema.columns WHERE table_schema=? ORDER BY table_name,ordinal_position`, dbName)
		if e != nil {
			return e
		}
		defer columns.Close()
		for columns.Next() {
			var table string
			var x models.ColumnInfo
			var nullable string
			var def sql.NullString
			if e = columns.Scan(&table, &x.Name, &x.DatabaseType, &x.ColumnType, &nullable, &x.Key, &def, &x.Extra); e != nil {
				return e
			}
			t := byName[table]
			if t == nil {
				continue
			}
			x.Nullable = nullable == "YES"
			if def.Valid {
				x.Default = &def.String
			}
			t.Columns = append(t.Columns, x)
		}
		if e = columns.Err(); e != nil {
			return e
		}
		fq, e := db.QueryContext(ctx, `SELECT table_name,constraint_name,column_name,referenced_table_name,referenced_column_name FROM information_schema.key_column_usage WHERE table_schema=? AND referenced_table_name IS NOT NULL ORDER BY table_name,constraint_name,ordinal_position`, dbName)
		if e != nil {
			return e
		}
		defer fq.Close()
		for fq.Next() {
			var table string
			var f models.ForeignKeyInfo
			if e = fq.Scan(&table, &f.Name, &f.Column, &f.ReferencedTable, &f.ReferencedColumn); e != nil {
				return e
			}
			t := byName[table]
			if t == nil {
				continue
			}
			t.ForeignKeys = append(t.ForeignKeys, f)
		}
		return fq.Err()
	})
	return
}
func (p *Provider) GetTableData(ctx context.Context, c models.Connection, dbName, table string, limit, offset int, sort, dir string) (*models.QueryResult, error) {
	qdb, err := database.QuoteIdentifier(dbName)
	if err != nil {
		return nil, err
	}
	qt, err := database.QuoteIdentifier(table)
	if err != nil {
		return nil, err
	}
	order := ""
	if sort != "" {
		qs, e := database.QuoteIdentifier(sort)
		if e != nil {
			return nil, e
		}
		d := strings.ToUpper(dir)
		if d != "ASC" && d != "DESC" {
			d = "ASC"
		}
		order = " ORDER BY " + qs + " " + d
	}
	// Fetch one extra row to tell the client whether another page exists. run
	// treats the explicit LIMIT as the caller's own cap and leaves the result
	// untouched, so this page is trimmed here.
	result, err := p.run(ctx, c, "SELECT * FROM "+qdb+"."+qt+order+" LIMIT ? OFFSET ?", limit, []any{limit + 1, offset})
	if err != nil {
		return nil, err
	}
	if len(result.Rows) > limit {
		result.Rows = result.Rows[:limit]
		result.HasMore = true
	}
	result.RowCount = len(result.Rows)
	return result, nil
}
func (p *Provider) Query(ctx context.Context, c models.Connection, statement string, maxRows int) (*models.QueryResult, error) {
	return p.run(ctx, c, statement, maxRows, nil)
}

// RestoreDatabase replaces a database with the contents of a SQL dump. It
// intentionally opens a server-level connection (without a default schema),
// because the selected database may be the connection's initial database and
// is therefore unavailable immediately after DROP DATABASE.
func (p *Provider) RestoreDatabase(ctx context.Context, c models.Connection, databaseName, script string) (*models.QueryResult, error) {
	quotedDatabase, err := database.QuoteIdentifier(databaseName)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	serverConnection := c
	serverConnection.InitialDatabase = ""
	db, err := p.open(serverConnection)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if _, err = db.ExecContext(ctx, "DROP DATABASE IF EXISTS "+quotedDatabase); err != nil {
		return nil, err
	}
	if _, err = db.ExecContext(ctx, "CREATE DATABASE "+quotedDatabase); err != nil {
		return nil, err
	}
	if _, err = db.ExecContext(ctx, "USE "+quotedDatabase); err != nil {
		return nil, err
	}
	if _, err = db.ExecContext(ctx, script); err != nil {
		return nil, err
	}
	result := newQueryResult()
	result.ExecutionTimeMs = time.Since(started).Milliseconds()
	return result, nil
}

// RecreateDatabase drops a database and creates it again empty, preserving its
// character set and collation. Like RestoreDatabase it uses a server-level
// connection, because the target may be the connection's initial schema.
func (p *Provider) RecreateDatabase(ctx context.Context, c models.Connection, databaseName string) (*models.QueryResult, error) {
	quotedDatabase, err := database.QuoteIdentifier(databaseName)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	serverConnection := c
	serverConnection.InitialDatabase = ""
	db, err := p.open(serverConnection)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var charset, collation sql.NullString
	row := db.QueryRowContext(ctx, "SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?", databaseName)
	if err = row.Scan(&charset, &collation); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	if _, err = db.ExecContext(ctx, "DROP DATABASE IF EXISTS "+quotedDatabase); err != nil {
		return nil, err
	}
	create := "CREATE DATABASE " + quotedDatabase
	if database.ValidateIdentifier(charset.String) == nil {
		create += " CHARACTER SET " + charset.String
	}
	if database.ValidateIdentifier(collation.String) == nil {
		create += " COLLATE " + collation.String
	}
	if _, err = db.ExecContext(ctx, create); err != nil {
		return nil, err
	}
	result := newQueryResult()
	result.ExecutionTimeMs = time.Since(started).Milliseconds()
	return result, nil
}

// UpdateRow updates only changed columns. The original result values form the
// WHERE predicate so a concurrently changed row is never overwritten silently.
func (p *Provider) UpdateRow(ctx context.Context, c models.Connection, dbName, table string, original, changes map[string]any) (*models.QueryResult, error) {
	statement, args, err := updateRowStatement(dbName, table, original, changes)
	if err != nil {
		return nil, err
	}
	return p.run(ctx, c, statement, 0, args)
}

func (p *Provider) UpdateRowInTransaction(ctx context.Context, c models.Connection, dbName, table string, original, changes map[string]any) (*models.QueryResult, error) {
	statement, args, err := updateRowStatement(dbName, table, original, changes)
	if err != nil {
		return nil, err
	}
	return p.runMutationInTransaction(ctx, c, statement, args)
}

// InsertRow adds one row using only the supplied columns. An empty value map
// intentionally uses MySQL defaults for every column.
func (p *Provider) InsertRow(ctx context.Context, c models.Connection, dbName, table string, values map[string]any) (*models.QueryResult, error) {
	statement, args, err := insertRowStatement(dbName, table, values)
	if err != nil {
		return nil, err
	}
	return p.run(ctx, c, statement, 0, args)
}

func (p *Provider) InsertRowInTransaction(ctx context.Context, c models.Connection, dbName, table string, values map[string]any) (*models.QueryResult, error) {
	statement, args, err := insertRowStatement(dbName, table, values)
	if err != nil {
		return nil, err
	}
	return p.runMutationInTransaction(ctx, c, statement, args)
}

func (p *Provider) DeleteRow(ctx context.Context, c models.Connection, dbName, table string, original map[string]any) (*models.QueryResult, error) {
	statement, args, err := deleteRowStatement(dbName, table, original)
	if err != nil {
		return nil, err
	}
	return p.run(ctx, c, statement, 0, args)
}

func (p *Provider) DeleteRowInTransaction(ctx context.Context, c models.Connection, dbName, table string, original map[string]any) (*models.QueryResult, error) {
	statement, args, err := deleteRowStatement(dbName, table, original)
	if err != nil {
		return nil, err
	}
	return p.runMutationInTransaction(ctx, c, statement, args)
}

func (p *Provider) runMutationInTransaction(ctx context.Context, c models.Connection, statement string, args []any) (*models.QueryResult, error) {
	return p.stageMutation(ctx, c, statement, args, 1)
}

// stageMutation deliberately does not execute SQL. MySQL implicitly commits
// several DDL statements (such as CREATE, ALTER, and DROP), so keeping a live
// transaction cannot protect a production connection from schema changes.
// Production changes are instead queued and only sent to MySQL on Commit.
func (p *Provider) stageMutation(ctx context.Context, c models.Connection, statement string, args []any, affectedRows int64) (*models.QueryResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.transactionMu.Lock()
	session := p.transactions[c.ID]
	if session == nil {
		session = &transactionSession{}
		p.transactions[c.ID] = session
	}
	session.mu.Lock()
	p.transactionMu.Unlock()
	defer session.mu.Unlock()

	session.addStatement(statement, args)
	result := newQueryResult()
	result.AffectedRows = affectedRows
	result.TransactionPending = true
	result.PendingStatements = len(session.statements)
	return result, nil
}

func updateRowStatement(dbName, table string, original, changes map[string]any) (string, []any, error) {
	if len(original) == 0 || len(changes) == 0 {
		return "", nil, fmt.Errorf("original row and changes are required")
	}
	qdb, err := database.QuoteIdentifier(dbName)
	if err != nil {
		return "", nil, err
	}
	qt, err := database.QuoteIdentifier(table)
	if err != nil {
		return "", nil, err
	}
	changeColumns, originalColumns := sortedKeys(changes), sortedKeys(original)
	set := make([]string, 0, len(changeColumns))
	where := make([]string, 0, len(originalColumns))
	args := make([]any, 0, len(changeColumns)+len(originalColumns))
	for _, column := range changeColumns {
		quoted, quoteErr := database.QuoteIdentifier(column)
		if quoteErr != nil {
			return "", nil, quoteErr
		}
		set = append(set, quoted+"=?")
		args = append(args, normalizeTemporalValue(changes[column]))
	}
	for _, column := range originalColumns {
		quoted, quoteErr := database.QuoteIdentifier(column)
		if quoteErr != nil {
			return "", nil, quoteErr
		}
		where = append(where, quoted+" <=> ?")
		args = append(args, normalizeTemporalValue(original[column]))
	}
	return "UPDATE " + qdb + "." + qt + " SET " + strings.Join(set, ", ") + " WHERE " + strings.Join(where, " AND ") + " LIMIT 1", args, nil
}

func insertRowStatement(dbName, table string, values map[string]any) (string, []any, error) {
	qdb, err := database.QuoteIdentifier(dbName)
	if err != nil {
		return "", nil, err
	}
	qt, err := database.QuoteIdentifier(table)
	if err != nil {
		return "", nil, err
	}
	columns := sortedKeys(values)
	if len(columns) == 0 {
		return "INSERT INTO " + qdb + "." + qt + " () VALUES ()", nil, nil
	}
	quoted := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))
	for _, column := range columns {
		name, quoteErr := database.QuoteIdentifier(column)
		if quoteErr != nil {
			return "", nil, quoteErr
		}
		quoted = append(quoted, name)
		args = append(args, normalizeTemporalValue(values[column]))
	}
	return "INSERT INTO " + qdb + "." + qt + " (" + strings.Join(quoted, ", ") + ") VALUES (" + strings.TrimRight(strings.Repeat("?, ", len(columns)), ", ") + ")", args, nil
}

func deleteRowStatement(dbName, table string, original map[string]any) (string, []any, error) {
	if len(original) == 0 {
		return "", nil, fmt.Errorf("original row is required")
	}
	qdb, err := database.QuoteIdentifier(dbName)
	if err != nil {
		return "", nil, err
	}
	qt, err := database.QuoteIdentifier(table)
	if err != nil {
		return "", nil, err
	}
	columns := sortedKeys(original)
	where := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))
	for _, column := range columns {
		name, quoteErr := database.QuoteIdentifier(column)
		if quoteErr != nil {
			return "", nil, quoteErr
		}
		where = append(where, name+" <=> ?")
		args = append(args, normalizeTemporalValue(original[column]))
	}
	return "DELETE FROM " + qdb + "." + qt + " WHERE " + strings.Join(where, " AND ") + " LIMIT 1", args, nil
}

// normalizeTemporalValue converts the RFC 3339 strings the API emits for
// DATE/DATETIME/TIMESTAMP columns back into MySQL's literal format. Sending
// "2026-08-04T23:14:46Z" straight back raises error 1292 under strict mode,
// even when the column only appears in the WHERE predicate. The wall clock is
// preserved so the value still matches what was read from the row.
func normalizeTemporalValue(value any) any {
	text, ok := value.(string)
	if !ok || !rfc3339Pattern.MatchString(text) {
		return value
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return value
	}
	return parsed.Format("2006-01-02 15:04:05.999999999")
}

var rfc3339Pattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[Tt]\d{2}:\d{2}:\d{2}(\.\d+)?([Zz]|[+-]\d{2}:\d{2})$`)

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// QueryInTransaction queues production mutations without executing them. Read-only
// statements still run immediately so a pending change can be reviewed safely.
func (p *Provider) QueryInTransaction(ctx context.Context, c models.Connection, statement string, maxRows int, mutating bool) (*models.QueryResult, error) {
	if mutating {
		return p.stageMutation(ctx, c, statement, nil, 0)
	}
	return p.Query(ctx, c, statement, maxRows)
}

func (p *Provider) TransactionStatus(c models.Connection) models.TransactionStatus {
	p.transactionMu.Lock()
	defer p.transactionMu.Unlock()
	session := p.transactions[c.ID]
	if session == nil {
		return models.TransactionStatus{}
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.status()
}

func (p *Provider) CommitTransaction(ctx context.Context, c models.Connection, statementIDs []string) (models.TransactionStatus, error) {
	return p.finishTransaction(ctx, c, statementIDs, true)
}

func (p *Provider) RollbackTransaction(ctx context.Context, c models.Connection, statementIDs []string) (models.TransactionStatus, error) {
	return p.finishTransaction(ctx, c, statementIDs, false)
}

// finishTransaction executes only the selected queued statements. Until this
// point no statement has been sent to MySQL, including DDL that cannot be
// safely enclosed in a MySQL transaction.
func (p *Provider) finishTransaction(ctx context.Context, c models.Connection, statementIDs []string, commit bool) (models.TransactionStatus, error) {
	p.transactionMu.Lock()
	session := p.transactions[c.ID]
	if session == nil {
		p.transactionMu.Unlock()
		return models.TransactionStatus{}, nil
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	defer p.transactionMu.Unlock()

	chosen, remaining, err := chooseStatements(session.statements, statementIDs)
	if err != nil {
		return session.status(), err
	}
	if commit && len(chosen) > 0 {
		if err := p.executeStatements(ctx, c, chosen, true); err != nil {
			// Keep the queue visible when MySQL rejects a statement, so the user
			// can revise or discard it instead of losing the pending work.
			return session.status(), err
		}
	}
	if len(remaining) == 0 {
		delete(p.transactions, c.ID)
		return models.TransactionStatus{}, nil
	}
	session.statements = remaining
	return session.status(), nil
}

func (s *transactionSession) addStatement(statement string, args []any) {
	s.nextID++
	s.statements = append(s.statements, pendingStatement{PendingTransactionStatement: models.PendingTransactionStatement{ID: fmt.Sprintf("pending-%d", s.nextID), SQL: statement}, args: args})
}

func (s *transactionSession) status() models.TransactionStatus {
	statements := make([]models.PendingTransactionStatement, len(s.statements))
	for i, statement := range s.statements {
		statements[i] = statement.PendingTransactionStatement
	}
	return models.TransactionStatus{Pending: len(statements) > 0, PendingStatements: len(statements), Statements: statements}
}

func chooseStatements(statements []pendingStatement, ids []string) (chosen, remaining []pendingStatement, err error) {
	if len(ids) == 0 {
		return append([]pendingStatement(nil), statements...), nil, nil
	}
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id != "" {
			wanted[id] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil, nil, fmt.Errorf("select at least one pending statement")
	}
	for _, statement := range statements {
		if _, ok := wanted[statement.ID]; ok {
			chosen = append(chosen, statement)
			delete(wanted, statement.ID)
		} else {
			remaining = append(remaining, statement)
		}
	}
	if len(wanted) > 0 {
		return nil, nil, fmt.Errorf("one or more pending statements no longer exist")
	}
	return chosen, remaining, nil
}

func (p *Provider) executeStatements(ctx context.Context, c models.Connection, statements []pendingStatement, commit bool) error {
	db, err := p.open(c)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := beginTransaction(ctx, db)
	if err != nil {
		return err
	}
	for _, statement := range statements {
		if _, err := p.runWithQueryer(ctx, statement.SQL, 0, statement.args, tx); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if commit {
		return tx.Commit()
	}
	return tx.Rollback()
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// beginTransaction deliberately detaches the transaction lifetime from the
// request context. The query request completes before the user presses Commit,
// and database/sql rolls a transaction back when its BeginTx context is
// cancelled.
func beginTransaction(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return db.BeginTx(context.WithoutCancel(ctx), nil)
}

// newQueryResult keeps collection fields as JSON arrays even for statements
// such as UPDATE that do not return a result set.
func newQueryResult() *models.QueryResult {
	return &models.QueryResult{
		Columns: []models.QueryColumn{},
		Rows:    []map[string]any{},
	}
}

func (p *Provider) run(ctx context.Context, c models.Connection, statement string, maxRows int, args []any) (result *models.QueryResult, err error) {
	started := time.Now()
	err = p.withDB(c, func(db *sql.DB) error {
		var e error
		result, e = p.runWithQueryer(ctx, statement, maxRows, args, db)
		return e
	})
	if result == nil {
		result = newQueryResult()
	}
	result.ExecutionTimeMs = time.Since(started).Milliseconds()
	return
}
func (p *Provider) runWithQueryer(ctx context.Context, statement string, maxRows int, args []any, db queryer) (result *models.QueryResult, err error) {
	started := time.Now()
	result = newQueryResult()
	if executesWithoutRows(statement) {
		exec, e := db.ExecContext(ctx, statement, args...)
		if e != nil {
			return result, e
		}
		a, _ := exec.RowsAffected()
		last, _ := exec.LastInsertId()
		result = newQueryResult()
		result.AffectedRows = a
		result.LastInsertID = last
		result.ExecutionTimeMs = time.Since(started).Milliseconds()
		return result, nil
	}
	rowLimit := maxRows
	if hasTopLevelLimit(statement) {
		// An explicit LIMIT is an intentional request from the user. Do not cap
		// either the SQL sent to MySQL or the rows returned to the editor.
		rowLimit = int(^uint(0) >> 1)
	} else {
		statement = limitSelectRows(statement, maxRows)
	}
	rows, e := db.QueryContext(ctx, statement, args...)
	if e != nil {
		exec, e2 := db.ExecContext(ctx, statement, args...)
		if e2 != nil {
			return result, e2
		}
		a, _ := exec.RowsAffected()
		last, _ := exec.LastInsertId()
		result = newQueryResult()
		result.AffectedRows = a
		result.LastInsertID = last
		result.ExecutionTimeMs = time.Since(started).Milliseconds()
		return result, nil
	}
	defer rows.Close()
	types, e := rows.ColumnTypes()
	if e != nil {
		return result, e
	}
	for _, t := range types {
		nullable, _ := t.Nullable()
		result.Columns = append(result.Columns, models.QueryColumn{Name: t.Name(), DatabaseType: t.DatabaseTypeName(), Nullable: nullable})
	}
	for rows.Next() {
		if len(result.Rows) >= rowLimit {
			result.HasMore = true
			break
		}
		values := make([]any, len(types))
		ptrs := make([]any, len(values))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if e = rows.Scan(ptrs...); e != nil {
			return result, e
		}
		item := map[string]any{}
		for i, col := range result.Columns {
			switch v := values[i].(type) {
			case []byte:
				item[col.Name] = string(v)
			case time.Time:
				item[col.Name] = v.Format(time.RFC3339Nano)
			default:
				item[col.Name] = v
			}
		}
		result.Rows = append(result.Rows, item)
	}
	if e = rows.Err(); e != nil {
		return result, e
	}
	if result.HasMore {
		result.Rows = result.Rows[:rowLimit]
	}
	result.RowCount = len(result.Rows)
	result.ExecutionTimeMs = time.Since(started).Milliseconds()
	return result, nil
}

func executesWithoutRows(statement string) bool {
	parts := strings.Fields(strings.ToUpper(strings.TrimSpace(statement)))
	if len(parts) == 0 {
		return false
	}
	return parts[0] == "INSERT" || parts[0] == "UPDATE" || parts[0] == "DELETE" || parts[0] == "SET"
}

// limitSelectRows makes the result cap effective in MySQL, rather than merely
// stopping the application after it has already received an unbounded result
// set. The extra row preserves the HasMore signal returned to the client.
func limitSelectRows(statement string, maxRows int) string {
	trimmed := strings.TrimSpace(statement)
	parts := strings.Fields(strings.ToUpper(trimmed))
	if maxRows < 1 || len(parts) == 0 || parts[0] != "SELECT" || hasTopLevelLimit(trimmed) {
		return statement
	}
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, ";"))
	return fmt.Sprintf("SELECT * FROM (%s) AS `dbfock_result` LIMIT %d", trimmed, maxRows+1)
}

// hasTopLevelLimit deliberately ignores LIMIT inside nested subqueries,
// strings, quoted identifiers, and comments. Only a limit on the query the
// user actually ran opts out of DBfock's default result cap.
func hasTopLevelLimit(statement string) bool {
	trimmed := strings.TrimSpace(statement)
	parts := strings.Fields(strings.ToUpper(trimmed))
	if len(parts) == 0 || parts[0] != "SELECT" {
		return false
	}

	depth := 0
	word := strings.Builder{}
	flushWord := func() bool {
		if depth == 0 && strings.EqualFold(word.String(), "LIMIT") {
			return true
		}
		word.Reset()
		return false
	}
	for i := 0; i < len(trimmed); i++ {
		ch := trimmed[i]
		if ch == '\'' || ch == '"' || ch == '`' {
			if flushWord() {
				return true
			}
			quote := ch
			for i++; i < len(trimmed); i++ {
				if trimmed[i] == '\\' && quote != '`' {
					i++
					continue
				}
				if trimmed[i] == quote {
					break
				}
			}
			continue
		}
		if ch == '-' && i+1 < len(trimmed) && trimmed[i+1] == '-' {
			if flushWord() {
				return true
			}
			for i++; i < len(trimmed) && trimmed[i] != '\n'; i++ {
			}
			continue
		}
		if ch == '#' {
			if flushWord() {
				return true
			}
			for i++; i < len(trimmed) && trimmed[i] != '\n'; i++ {
			}
			continue
		}
		if ch == '/' && i+1 < len(trimmed) && trimmed[i+1] == '*' {
			if flushWord() {
				return true
			}
			for i += 2; i+1 < len(trimmed) && !(trimmed[i] == '*' && trimmed[i+1] == '/'); i++ {
			}
			i++
			continue
		}
		switch ch {
		case '(':
			if flushWord() {
				return true
			}
			depth++
		case ')':
			if flushWord() {
				return true
			}
			if depth > 0 {
				depth--
			}
		default:
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
				word.WriteByte(ch)
			} else if flushWord() {
				return true
			}
		}
	}
	return flushWord()
}
