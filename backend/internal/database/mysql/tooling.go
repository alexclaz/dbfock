package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dbfock/database-manager/backend/internal/database"
	"github.com/dbfock/database-manager/backend/internal/models"
)

type schemaColumn struct {
	name, columnType, key, extra string
	nullable                     bool
	defaultValue                 *string
}
type schemaTable map[string]schemaColumn
type schemaSnapshot map[string]map[string]schemaTable

func (p *Provider) CompareSchemas(ctx context.Context, source, target models.Connection) (*models.SchemaComparison, error) {
	sourceDB, err := p.open(source)
	if err != nil {
		return nil, err
	}
	defer sourceDB.Close()
	targetDB, err := p.open(target)
	if err != nil {
		return nil, err
	}
	defer targetDB.Close()

	sourceSchema, err := readSchemaSnapshot(ctx, sourceDB)
	if err != nil {
		return nil, fmt.Errorf("read source schema: %w", err)
	}
	targetSchema, err := readSchemaSnapshot(ctx, targetDB)
	if err != nil {
		return nil, fmt.Errorf("read target schema: %w", err)
	}

	result := &models.SchemaComparison{SourceConnectionID: source.ID, TargetConnectionID: target.ID, Databases: []models.DatabaseSchemaDifference{}}
	databaseNames := unionKeys(sourceSchema, targetSchema)
	for _, databaseName := range databaseNames {
		sourceTables, sourceOK := sourceSchema[databaseName]
		targetTables, targetOK := targetSchema[databaseName]
		result.Summary.DatabasesCompared++
		databaseDiff := models.DatabaseSchemaDifference{Name: databaseName, Status: "different", Tables: []models.TableSchemaDifference{}}
		if !sourceOK {
			databaseDiff.Status = "missing_source"
		}
		if !targetOK {
			databaseDiff.Status = "missing_target"
		}
		for _, tableName := range unionKeys(sourceTables, targetTables) {
			result.Summary.TablesCompared++
			sourceColumns, tableInSource := sourceTables[tableName]
			targetColumns, tableInTarget := targetTables[tableName]
			tableDiff := models.TableSchemaDifference{Name: tableName, Status: "different", Columns: []models.ColumnSchemaDifference{}}
			if !tableInSource {
				tableDiff.Status = "missing_source"
			}
			if !tableInTarget {
				tableDiff.Status = "missing_target"
			}
			if tableInSource && tableInTarget {
				for _, columnName := range unionKeys(sourceColumns, targetColumns) {
					sourceColumn, columnInSource := sourceColumns[columnName]
					targetColumn, columnInTarget := targetColumns[columnName]
					status := "different"
					if !columnInSource {
						status = "missing_source"
					} else if !columnInTarget {
						status = "missing_target"
					} else if columnsEqual(sourceColumn, targetColumn) {
						continue
					}
					tableDiff.Columns = append(tableDiff.Columns, columnDifference(columnName, status, sourceColumn, targetColumn, columnInSource, columnInTarget))
				}
				if len(tableDiff.Columns) == 0 {
					result.Summary.EqualTables++
					continue
				}
			}
			result.Summary.DifferentTables++
			databaseDiff.Tables = append(databaseDiff.Tables, tableDiff)
		}
		if len(databaseDiff.Tables) > 0 || !sourceOK || !targetOK {
			result.Databases = append(result.Databases, databaseDiff)
		}
	}
	return result, nil
}

func readSchemaSnapshot(ctx context.Context, db *sql.DB) (schemaSnapshot, error) {
	result := schemaSnapshot{}
	databases, err := db.QueryContext(ctx, `SELECT schema_name FROM information_schema.schemata
		WHERE schema_name NOT IN ('information_schema','mysql','performance_schema','sys') ORDER BY schema_name`)
	if err != nil {
		return nil, err
	}
	for databases.Next() {
		var databaseName string
		if err = databases.Scan(&databaseName); err != nil {
			databases.Close()
			return nil, err
		}
		result[databaseName] = map[string]schemaTable{}
	}
	if err = databases.Err(); err != nil {
		databases.Close()
		return nil, err
	}
	databases.Close()

	rows, err := db.QueryContext(ctx, `SELECT table_schema, table_name, column_name, column_type, is_nullable, column_default, column_key, extra
		FROM information_schema.columns
		WHERE table_schema NOT IN ('information_schema','mysql','performance_schema','sys')
		ORDER BY table_schema, table_name, ordinal_position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var databaseName, tableName, columnName, columnType, nullable, key, extra string
		var defaultValue sql.NullString
		if err = rows.Scan(&databaseName, &tableName, &columnName, &columnType, &nullable, &defaultValue, &key, &extra); err != nil {
			return nil, err
		}
		if result[databaseName] == nil {
			result[databaseName] = map[string]schemaTable{}
		}
		if result[databaseName][tableName] == nil {
			result[databaseName][tableName] = schemaTable{}
		}
		var defaultPointer *string
		if defaultValue.Valid {
			value := defaultValue.String
			defaultPointer = &value
		}
		result[databaseName][tableName][columnName] = schemaColumn{name: columnName, columnType: columnType, nullable: nullable == "YES", defaultValue: defaultPointer, key: key, extra: extra}
	}
	return result, rows.Err()
}

func unionKeys[A any](first, second map[string]A) []string {
	keys := make(map[string]bool, len(first)+len(second))
	for key := range first {
		keys[key] = true
	}
	for key := range second {
		keys[key] = true
	}
	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func columnsEqual(first, second schemaColumn) bool {
	return first.columnType == second.columnType && first.nullable == second.nullable && stringPointersEqual(first.defaultValue, second.defaultValue) && first.key == second.key && first.extra == second.extra
}
func stringPointersEqual(first, second *string) bool {
	if first == nil || second == nil {
		return first == second
	}
	return *first == *second
}
func columnDifference(name, status string, source, target schemaColumn, sourceOK, targetOK bool) models.ColumnSchemaDifference {
	result := models.ColumnSchemaDifference{Name: name, Status: status}
	if sourceOK {
		result.SourceType, result.SourceNull, result.SourceDefault, result.SourceExtra, result.SourceKey = source.columnType, &source.nullable, source.defaultValue, source.extra, source.key
	}
	if targetOK {
		result.TargetType, result.TargetNull, result.TargetDefault, result.TargetExtra, result.TargetKey = target.columnType, &target.nullable, target.defaultValue, target.extra, target.key
	}
	return result
}

type migrationTable struct {
	name          string
	sizeBytes     int64
	estimatedRows int64
}
type migrationColumn struct {
	quotedName string
	dataType   string
}

func validateMigrationOptions(options models.DatabaseMigrationOptions) error {
	if options.Strategy != "drop_recreate" && options.Strategy != "truncate_insert" && options.Strategy != "merge" {
		return fmt.Errorf("unsupported migration strategy")
	}
	if options.MaxTableSizeBytes <= 0 {
		return fmt.Errorf("maximum table size must be greater than zero")
	}
	if len(options.Databases) == 0 {
		return fmt.Errorf("select at least one database")
	}
	if options.TargetDatabase != "" {
		if len(options.Databases) != 1 {
			return fmt.Errorf("a target database can only receive one source database")
		}
		if err := database.ValidateIdentifier(options.TargetDatabase); err != nil {
			return err
		}
	}
	return nil
}

func migrationTargetDatabase(options models.DatabaseMigrationOptions, sourceDatabase string) string {
	if options.TargetDatabase != "" {
		return options.TargetDatabase
	}
	return sourceDatabase
}

func (p *Provider) PlanMigration(ctx context.Context, source, _ models.Connection, options models.DatabaseMigrationOptions) (*models.DatabaseMigrationPlan, error) {
	if err := validateMigrationOptions(options); err != nil {
		return nil, err
	}
	sourceDB, err := p.open(source)
	if err != nil {
		return nil, err
	}
	defer sourceDB.Close()
	plan := &models.DatabaseMigrationPlan{Databases: len(options.Databases), Tables: []models.DatabaseMigrationTable{}, SkippedTables: []models.DatabaseMigrationSkip{}}
	for _, databaseName := range options.Databases {
		var databaseCount int
		if err = sourceDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?", databaseName).Scan(&databaseCount); err != nil {
			return nil, err
		}
		if databaseCount == 0 {
			return nil, fmt.Errorf("source database %s does not exist", databaseName)
		}
		tables, listErr := migrationTables(ctx, sourceDB, databaseName)
		if listErr != nil {
			return nil, fmt.Errorf("list tables in %s: %w", databaseName, listErr)
		}
		for _, table := range tables {
			if table.sizeBytes > options.MaxTableSizeBytes {
				plan.SkippedTables = append(plan.SkippedTables, models.DatabaseMigrationSkip{Database: databaseName, Table: table.name, SizeBytes: table.sizeBytes, Reason: "size_limit"})
				continue
			}
			plan.Tables = append(plan.Tables, models.DatabaseMigrationTable{Database: databaseName, Table: table.name, SizeBytes: table.sizeBytes, EstimatedRows: table.estimatedRows})
			plan.EstimatedRows += table.estimatedRows
			plan.TotalSizeBytes += table.sizeBytes
		}
	}
	return plan, nil
}

func (p *Provider) MigrateDatabases(ctx context.Context, source, target models.Connection, options models.DatabaseMigrationOptions, reportProgress func(models.DatabaseMigrationProgress)) (*models.DatabaseMigrationResult, error) {
	if err := validateMigrationOptions(options); err != nil {
		return nil, err
	}
	started := time.Now()
	sourceDB, err := p.open(source)
	if err != nil {
		return nil, err
	}
	defer sourceDB.Close()
	targetDB, err := p.open(target)
	if err != nil {
		return nil, err
	}
	defer targetDB.Close()
	targetConn, err := targetDB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer targetConn.Close()
	var originalSQLMode string
	if err = targetConn.QueryRowContext(ctx, "SELECT @@SESSION.sql_mode").Scan(&originalSQLMode); err != nil {
		return nil, err
	}
	relaxedSQLMode := migrationSQLMode(originalSQLMode)
	if relaxedSQLMode != originalSQLMode {
		if _, err = targetConn.ExecContext(ctx, "SET SESSION sql_mode = ?", relaxedSQLMode); err != nil {
			return nil, fmt.Errorf("allow legacy zero dates during migration: %w", err)
		}
		defer func() {
			restoreContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = targetConn.ExecContext(restoreContext, "SET SESSION sql_mode = ?", originalSQLMode)
		}()
	}
	if _, err = targetConn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
		return nil, err
	}
	defer targetConn.ExecContext(context.Background(), "SET FOREIGN_KEY_CHECKS=1")

	result := &models.DatabaseMigrationResult{SkippedTables: []models.DatabaseMigrationSkip{}, FailedTables: []models.DatabaseMigrationFailure{}}
	tablesByDatabase := make(map[string][]migrationTable, len(options.Databases))
	totalTables := 0
	for _, databaseName := range options.Databases {
		var databaseCount int
		if err = sourceDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?", databaseName).Scan(&databaseCount); err != nil {
			return nil, err
		}
		if databaseCount == 0 {
			return nil, fmt.Errorf("source database %s does not exist", databaseName)
		}
		tables, listErr := migrationTables(ctx, sourceDB, databaseName)
		if listErr != nil {
			return nil, fmt.Errorf("list tables in %s: %w", databaseName, listErr)
		}
		for _, table := range tables {
			if table.sizeBytes > options.MaxTableSizeBytes {
				result.SkippedTables = append(result.SkippedTables, models.DatabaseMigrationSkip{Database: databaseName, Table: table.name, SizeBytes: table.sizeBytes, Reason: "size_limit"})
				continue
			}
			tablesByDatabase[databaseName] = append(tablesByDatabase[databaseName], table)
			totalTables++
		}
	}
	progress := models.DatabaseMigrationProgress{TotalTables: totalTables}
	if reportProgress != nil {
		reportProgress(progress)
	}
	if options.RecreateTarget {
		sourceDatabase := options.Databases[0]
		targetDatabase := migrationTargetDatabase(options, sourceDatabase)
		quotedTarget, quoteErr := database.QuoteIdentifier(targetDatabase)
		if quoteErr != nil {
			return nil, quoteErr
		}
		var characterSet, collation string
		if err = sourceDB.QueryRowContext(ctx, `SELECT default_character_set_name, default_collation_name FROM information_schema.schemata WHERE schema_name = ?`, sourceDatabase).Scan(&characterSet, &collation); err != nil {
			return nil, fmt.Errorf("read source database settings: %w", err)
		}
		quotedCharacterSet, quoteErr := database.QuoteIdentifier(characterSet)
		if quoteErr != nil {
			return nil, quoteErr
		}
		quotedCollation, quoteErr := database.QuoteIdentifier(collation)
		if quoteErr != nil {
			return nil, quoteErr
		}
		if _, err = targetConn.ExecContext(ctx, "DROP DATABASE IF EXISTS "+quotedTarget); err != nil {
			return nil, err
		}
		if _, err = targetConn.ExecContext(ctx, "CREATE DATABASE "+quotedTarget+" CHARACTER SET "+quotedCharacterSet+" COLLATE "+quotedCollation); err != nil {
			return nil, err
		}
	}
	for _, databaseName := range options.Databases {
		targetDatabase := migrationTargetDatabase(options, databaseName)
		quotedDatabase, quoteErr := database.QuoteIdentifier(targetDatabase)
		if quoteErr != nil {
			return nil, quoteErr
		}
		if _, err = targetConn.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS "+quotedDatabase); err != nil {
			return nil, err
		}
		for _, table := range tablesByDatabase[databaseName] {
			progress.CurrentDatabase, progress.CurrentTable = databaseName, table.name
			progress.CurrentTableRows, progress.CurrentTableEstimatedRows = 0, table.estimatedRows
			if reportProgress != nil {
				reportProgress(progress)
			}
			rowsBeforeTable := result.RowsMigrated
			rowsCopied, migrateErr := migrateTable(ctx, sourceDB, targetConn, databaseName, targetDatabase, table.name, options, func(tableRows int64) {
				progress.RowsMigrated = rowsBeforeTable + tableRows
				progress.CurrentTableRows = tableRows
				if reportProgress != nil {
					reportProgress(progress)
				}
			})
			if migrateErr != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				failureMessage := migrateErr.Error()
				quotedTable, quoteErr := database.QuoteIdentifier(table.name)
				if quoteErr == nil && (options.Strategy != "merge" || options.RecreateTarget) {
					if _, cleanupErr := targetConn.ExecContext(ctx, "TRUNCATE TABLE "+quotedDatabase+"."+quotedTable); cleanupErr != nil {
						failureMessage += "; cleanup failed: " + cleanupErr.Error()
					}
				}
				result.FailedTables = append(result.FailedTables, models.DatabaseMigrationFailure{Database: databaseName, Table: table.name, Error: failureMessage})
				progress.CompletedTables++
				progress.RowsMigrated = result.RowsMigrated
				progress.CurrentTableRows, progress.CurrentTableEstimatedRows = 0, 0
				if reportProgress != nil {
					reportProgress(progress)
				}
				continue
			}
			result.TablesMigrated++
			result.RowsMigrated += rowsCopied
			progress.CompletedTables++
			progress.RowsMigrated = result.RowsMigrated
			progress.CurrentTableRows, progress.CurrentTableEstimatedRows = 0, 0
			if reportProgress != nil {
				reportProgress(progress)
			}
		}
		result.DatabasesMigrated++
	}
	result.ExecutionTimeMs = time.Since(started).Milliseconds()
	return result, nil
}

func migrationSQLMode(current string) string {
	kept := make([]string, 0)
	for _, mode := range strings.Split(current, ",") {
		mode = strings.TrimSpace(mode)
		if strings.EqualFold(mode, "NO_ZERO_DATE") || strings.EqualFold(mode, "NO_ZERO_IN_DATE") || mode == "" {
			continue
		}
		kept = append(kept, mode)
	}
	return strings.Join(kept, ",")
}

func migrationTables(ctx context.Context, db *sql.DB, databaseName string) ([]migrationTable, error) {
	rows, err := db.QueryContext(ctx, `SELECT table_name, COALESCE(data_length, 0), COALESCE(table_rows, 0) FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE' ORDER BY table_name`, databaseName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []migrationTable{}
	for rows.Next() {
		var table migrationTable
		if err = rows.Scan(&table.name, &table.sizeBytes, &table.estimatedRows); err != nil {
			return nil, err
		}
		result = append(result, table)
	}
	return result, rows.Err()
}

func migrateTable(ctx context.Context, sourceDB *sql.DB, target *sql.Conn, sourceDatabase, targetDatabase, tableName string, options models.DatabaseMigrationOptions, reportRows func(int64)) (int64, error) {
	quotedSourceDatabase, err := database.QuoteIdentifier(sourceDatabase)
	if err != nil {
		return 0, err
	}
	quotedTargetDatabase, err := database.QuoteIdentifier(targetDatabase)
	if err != nil {
		return 0, err
	}
	quotedTable, err := database.QuoteIdentifier(tableName)
	if err != nil {
		return 0, err
	}
	sourceQualified := quotedSourceDatabase + "." + quotedTable
	targetQualified := quotedTargetDatabase + "." + quotedTable
	createTable := func(ifNotExists bool) error {
		var ignoredName, ddl string
		if createErr := sourceDB.QueryRowContext(ctx, "SHOW CREATE TABLE "+sourceQualified).Scan(&ignoredName, &ddl); createErr != nil {
			return createErr
		}
		if ifNotExists {
			ddl = strings.Replace(ddl, "CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ", 1)
		}
		if _, createErr := target.ExecContext(ctx, "USE "+quotedTargetDatabase); createErr != nil {
			return createErr
		}
		_, createErr := target.ExecContext(ctx, ddl)
		return createErr
	}
	if options.Strategy == "drop_recreate" || options.Strategy == "merge" {
		if options.Strategy == "drop_recreate" {
			if _, err = target.ExecContext(ctx, "DROP TABLE IF EXISTS "+targetQualified); err != nil {
				return 0, err
			}
		}
		if err = createTable(options.Strategy == "merge"); err != nil {
			return 0, err
		}
	} else {
		var targetTableCount int
		if err = target.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = ? AND table_type = 'BASE TABLE'`, targetDatabase, tableName).Scan(&targetTableCount); err != nil {
			return 0, err
		}
		if targetTableCount == 0 {
			if !options.CreateMissing {
				return 0, fmt.Errorf("target table %s.%s does not exist", targetDatabase, tableName)
			}
			if err = createTable(false); err != nil {
				return 0, err
			}
		} else if _, err = target.ExecContext(ctx, "TRUNCATE TABLE "+targetQualified); err != nil {
			return 0, err
		}
	}
	if options.StructureOnly {
		return 0, nil
	}

	// Generated columns must not be present in an INSERT. Reading the explicit
	// column list also makes truncate/insert work when the destination has extra
	// nullable columns that are not present at the source.
	var targetColumns map[string]bool
	if options.Strategy == "merge" {
		targetColumns = map[string]bool{}
		targetColumnRows, targetErr := target.QueryContext(ctx, `SELECT column_name FROM information_schema.columns
			WHERE table_schema = ? AND table_name = ?
			  AND extra NOT LIKE '%VIRTUAL GENERATED%' AND extra NOT LIKE '%STORED GENERATED%'`, targetDatabase, tableName)
		if targetErr != nil {
			return 0, targetErr
		}
		for targetColumnRows.Next() {
			var columnName string
			if targetErr = targetColumnRows.Scan(&columnName); targetErr != nil {
				targetColumnRows.Close()
				return 0, targetErr
			}
			targetColumns[columnName] = true
		}
		if targetErr = targetColumnRows.Err(); targetErr != nil {
			targetColumnRows.Close()
			return 0, targetErr
		}
		targetColumnRows.Close()
	}
	columnRows, err := sourceDB.QueryContext(ctx, `SELECT column_name, data_type FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ?
		  AND extra NOT LIKE '%VIRTUAL GENERATED%' AND extra NOT LIKE '%STORED GENERATED%'
		ORDER BY ordinal_position`, sourceDatabase, tableName)
	if err != nil {
		return 0, err
	}
	columns := []migrationColumn{}
	for columnRows.Next() {
		var columnName, dataType string
		if err = columnRows.Scan(&columnName, &dataType); err != nil {
			columnRows.Close()
			return 0, err
		}
		if targetColumns != nil && !targetColumns[columnName] {
			continue
		}
		quotedColumn, quoteErr := database.QuoteIdentifier(columnName)
		if quoteErr != nil {
			columnRows.Close()
			return 0, quoteErr
		}
		columns = append(columns, migrationColumn{quotedName: quotedColumn, dataType: dataType})
	}
	if err = columnRows.Err(); err != nil {
		columnRows.Close()
		return 0, err
	}
	columnRows.Close()
	if len(columns) == 0 {
		return 0, nil
	}
	columnNames := make([]string, len(columns))
	for index := range columns {
		columnNames[index] = columns[index].quotedName
	}
	rows, err := sourceDB.QueryContext(ctx, "SELECT "+strings.Join(columnNames, ",")+" FROM "+sourceQualified)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	values, pointers := make([]any, len(columns)), make([]any, len(columns))
	for index := range values {
		pointers[index] = &values[index]
	}
	// Stay below both MySQL's prepared-statement placeholder limit and the
	// usual max_allowed_packet. A single unusually large row is still sent on
	// its own so the server can return a precise packet-size error.
	batchRowLimit := min(250, 60000/len(columns))
	args := make([]any, 0, len(columns)*batchRowLimit)
	rowCount, rowsInBatch, batchBytes := int64(0), 0, 0
	flush := func() error {
		if rowsInBatch == 0 {
			return nil
		}
		rowPlaceholder := "(" + strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",") + ")"
		insert := "INSERT INTO "
		if options.IgnoreDuplicates {
			insert = "INSERT IGNORE INTO "
		}
		statement := insert + targetQualified + " (" + strings.Join(columnNames, ",") + ") VALUES " + strings.TrimSuffix(strings.Repeat(rowPlaceholder+",", rowsInBatch), ",")
		_, execErr := target.ExecContext(ctx, statement, args...)
		if execErr == nil && reportRows != nil {
			reportRows(rowCount)
		}
		args = args[:0]
		rowsInBatch, batchBytes = 0, 0
		return execErr
	}
	for rows.Next() {
		if err = rows.Scan(pointers...); err != nil {
			return rowCount, err
		}
		for index := range values {
			values[index] = migrationValue(values[index], columns[index].dataType)
		}
		rowBytes := estimatedValuesSize(values)
		if rowsInBatch > 0 && batchBytes+rowBytes > insertBatchBytes {
			if err = flush(); err != nil {
				return rowCount, err
			}
		}
		args = append(args, values...)
		rowsInBatch++
		batchBytes += rowBytes
		rowCount++
		if rowsInBatch == batchRowLimit {
			if err = flush(); err != nil {
				return rowCount, err
			}
		}
	}
	if err = rows.Err(); err != nil {
		return rowCount, err
	}
	if err = flush(); err != nil {
		return rowCount, err
	}
	return rowCount, nil
}

// MySQL's driver exposes JSON and most text-like values as []byte when rows
// are scanned into interface values. Passing that []byte to a JSON column marks
// it with the binary character set, which MySQL rejects (error 3144). Strings
// preserve the connection character set; genuinely binary types stay bytes.
func migrationValue(value any, dataType string) any {
	bytes, ok := value.([]byte)
	if !ok || isBinaryType(dataType) {
		return value
	}
	return string(bytes)
}

func estimatedValuesSize(values []any) int {
	size := len(values) * 8
	for _, value := range values {
		switch typed := value.(type) {
		case []byte:
			size += len(typed)
		case string:
			size += len(typed)
		case time.Time:
			size += 32
		default:
			size += 24
		}
	}
	return size
}
