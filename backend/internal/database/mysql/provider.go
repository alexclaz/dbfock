package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dbfock/database-manager/backend/internal/database"
	"github.com/dbfock/database-manager/backend/internal/models"
	driver "github.com/go-sql-driver/mysql"
)

type transactionSession struct {
	db         *sql.DB
	tx         *sql.Tx
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
	// Fetch one extra row so run can tell the client whether another page exists,
	// while still returning at most limit rows to the caller.
	return p.run(ctx, c, "SELECT * FROM "+qdb+"."+qt+order+" LIMIT ? OFFSET ?", limit, []any{limit + 1, offset})
}
func (p *Provider) Query(ctx context.Context, c models.Connection, statement string, maxRows int) (*models.QueryResult, error) {
	return p.run(ctx, c, statement, maxRows, nil)
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
	p.transactionMu.Lock()
	session := p.transactions[c.ID]
	if session == nil {
		db, openErr := p.open(c)
		if openErr != nil {
			p.transactionMu.Unlock()
			return nil, openErr
		}
		tx, beginErr := beginTransaction(ctx, db)
		if beginErr != nil {
			db.Close()
			p.transactionMu.Unlock()
			return nil, beginErr
		}
		session = &transactionSession{db: db, tx: tx}
		p.transactions[c.ID] = session
	}
	session.mu.Lock()
	p.transactionMu.Unlock()
	defer session.mu.Unlock()

	result, err := p.runWithQueryer(ctx, statement, 0, args, session.tx)
	if err == nil && result.AffectedRows > 0 {
		session.addStatement(statement, args)
	}
	if result != nil {
		result.TransactionPending = len(session.statements) > 0
		result.PendingStatements = len(session.statements)
	}
	return result, err
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
		args = append(args, changes[column])
	}
	for _, column := range originalColumns {
		quoted, quoteErr := database.QuoteIdentifier(column)
		if quoteErr != nil {
			return "", nil, quoteErr
		}
		where = append(where, quoted+" <=> ?")
		args = append(args, original[column])
	}
	return "UPDATE " + qdb + "." + qt + " SET " + strings.Join(set, ", ") + " WHERE " + strings.Join(where, " AND ") + " LIMIT 1", args, nil
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// QueryInTransaction executes a statement on the connection's pending manual
// transaction. The transaction is intentionally kept open until commit or rollback.
func (p *Provider) QueryInTransaction(ctx context.Context, c models.Connection, statement string, maxRows int, mutating bool) (*models.QueryResult, error) {
	p.transactionMu.Lock()
	session := p.transactions[c.ID]
	if session == nil {
		db, err := p.open(c)
		if err != nil {
			p.transactionMu.Unlock()
			return nil, err
		}
		tx, err := beginTransaction(ctx, db)
		if err != nil {
			db.Close()
			p.transactionMu.Unlock()
			return nil, err
		}
		session = &transactionSession{db: db, tx: tx}
		p.transactions[c.ID] = session
	}
	session.mu.Lock()
	p.transactionMu.Unlock()
	defer session.mu.Unlock()

	result, err := p.runWithQueryer(ctx, statement, maxRows, nil, session.tx)
	if err == nil && mutating {
		session.addStatement(statement, nil)
	}
	if result != nil {
		result.TransactionPending = len(session.statements) > 0
		result.PendingStatements = len(session.statements)
	}
	return result, err
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

// finishTransaction supports both the original all-or-nothing controls and a
// chosen subset. A database transaction cannot commit or roll back an arbitrary
// statement independently, so for a subset we discard the current transaction,
// commit the chosen statements in order, and recreate the remaining pending
// transaction from the recorded statements.
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
	if len(remaining) == 0 {
		var finishErr error
		if commit {
			finishErr = session.tx.Commit()
		} else {
			finishErr = session.tx.Rollback()
		}
		delete(p.transactions, c.ID)
		closeErr := session.db.Close()
		if finishErr != nil && finishErr != sql.ErrTxDone {
			return models.TransactionStatus{}, finishErr
		}
		return models.TransactionStatus{}, closeErr
	}

	if err := session.tx.Rollback(); err != nil && err != sql.ErrTxDone {
		return session.status(), err
	}
	if err := session.db.Close(); err != nil {
		return session.status(), err
	}
	delete(p.transactions, c.ID)

	if commit {
		if err := p.executeStatements(ctx, c, chosen, true); err != nil {
			return p.restorePending(ctx, c, session.statements, session.nextID, err)
		}
	}
	if err := p.createPendingSession(ctx, c, remaining, session.nextID); err != nil {
		if !commit {
			return p.restorePending(ctx, c, session.statements, session.nextID, err)
		}
		return models.TransactionStatus{}, fmt.Errorf("selected changes were committed, but the remaining pending changes could not be restored: %w", err)
	}
	return p.transactions[c.ID].status(), nil
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

func (p *Provider) createPendingSession(ctx context.Context, c models.Connection, statements []pendingStatement, nextID int) error {
	db, err := p.open(c)
	if err != nil {
		return err
	}
	tx, err := beginTransaction(ctx, db)
	if err != nil {
		db.Close()
		return err
	}
	for _, statement := range statements {
		if _, err := p.runWithQueryer(ctx, statement.SQL, 0, statement.args, tx); err != nil {
			_ = tx.Rollback()
			db.Close()
			return err
		}
	}
	p.transactions[c.ID] = &transactionSession{db: db, tx: tx, statements: statements, nextID: nextID}
	return nil
}

func (p *Provider) restorePending(ctx context.Context, c models.Connection, statements []pendingStatement, nextID int, originalErr error) (models.TransactionStatus, error) {
	if err := p.createPendingSession(ctx, c, statements, nextID); err != nil {
		return models.TransactionStatus{}, fmt.Errorf("%w; could not restore pending changes: %v", originalErr, err)
	}
	return p.transactions[c.ID].status(), originalErr
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
	return parts[0] == "INSERT" || parts[0] == "UPDATE" || parts[0] == "DELETE"
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
