package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dbfock/database-manager/backend/internal/models"
	_ "modernc.org/sqlite"
)

const LocalUserID = "local-user"

type Repository struct{ db *sql.DB }

func Open(path string) (*Repository, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil && filepath.Dir(path) != "." {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return &Repository{db: db}, nil
}
func (r *Repository) Close() error { return r.db.Close() }
func (r *Repository) Migrate(ctx context.Context, migrationsPath string) error {
	if _, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at DATETIME NOT NULL)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	entries, err := os.ReadDir(migrationsPath)
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		var applied int
		if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, e.Name()).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", e.Name(), err)
		}
		if applied > 0 {
			continue
		}
		b, err := os.ReadFile(filepath.Join(migrationsPath, e.Name()))
		if err != nil {
			return err
		}
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin %s: %w", e.Name(), err)
		}
		if _, err = tx.ExecContext(ctx, string(b)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply %s: %w", e.Name(), err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, e.Name(), time.Now().UTC()); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record %s: %w", e.Name(), err)
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", e.Name(), err)
		}
	}
	now := time.Now().UTC()
	_, err = r.db.ExecContext(ctx, `INSERT OR IGNORE INTO users (id,name,email,password_hash,created_at,updated_at) VALUES (?,?,?,?,?,?)`, LocalUserID, "Local user", "local@dbfock.local", "!", now, now)
	return err
}

// MigrateFS applies embedded migrations. It lets distributable binaries carry their
// schema with them instead of requiring a working-directory-relative migrations folder.
func (r *Repository) MigrateFS(ctx context.Context, migrations fs.FS) error {
	if _, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at DATETIME NOT NULL)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	entries, err := fs.ReadDir(migrations, ".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		var applied int
		if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, entry.Name()).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", entry.Name(), err)
		}
		if applied > 0 {
			continue
		}
		contents, err := fs.ReadFile(migrations, entry.Name())
		if err != nil {
			return err
		}
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin %s: %w", entry.Name(), err)
		}
		if _, err = tx.ExecContext(ctx, string(contents)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply %s: %w", entry.Name(), err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, entry.Name(), time.Now().UTC()); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record %s: %w", entry.Name(), err)
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", entry.Name(), err)
		}
	}
	now := time.Now().UTC()
	_, err = r.db.ExecContext(ctx, `INSERT OR IGNORE INTO users (id,name,email,password_hash,created_at,updated_at) VALUES (?,?,?,?,?,?)`, LocalUserID, "Local user", "local@dbfock.local", "!", now, now)
	return err
}
func id() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func (r *Repository) CreateConnection(ctx context.Context, c models.Connection) (models.Connection, error) {
	newID, err := id()
	if err != nil {
		return c, err
	}
	c.ID = newID
	c.UserID = LocalUserID
	c.CreatedAt = time.Now().UTC()
	c.UpdatedAt = c.CreatedAt
	_, err = r.db.ExecContext(ctx, `INSERT INTO connections (id,user_id,name,driver,host,port,username,password_encrypted,initial_database,color,environment,ssl_enabled,timeout_seconds,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, c.ID, c.UserID, c.Name, c.Driver, c.Host, c.Port, c.Username, c.PasswordEncrypted, c.InitialDatabase, c.Color, c.Environment, c.SSLEnabled, c.TimeoutSeconds, c.CreatedAt, c.UpdatedAt)
	return c, err
}
func scanConnection(row interface{ Scan(...any) error }) (models.Connection, error) {
	var c models.Connection
	err := row.Scan(&c.ID, &c.UserID, &c.Name, &c.Driver, &c.Host, &c.Port, &c.Username, &c.PasswordEncrypted, &c.InitialDatabase, &c.Color, &c.Environment, &c.SSLEnabled, &c.TimeoutSeconds, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

const connectionFields = `id,user_id,name,driver,host,port,username,password_encrypted,initial_database,color,environment,ssl_enabled,timeout_seconds,created_at,updated_at`

func (r *Repository) ListConnections(ctx context.Context, userID string) ([]models.Connection, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+connectionFields+" FROM connections WHERE user_id=? ORDER BY name", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Connection, 0)
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (r *Repository) GetConnection(ctx context.Context, userID, id string) (models.Connection, error) {
	return scanConnection(r.db.QueryRowContext(ctx, "SELECT "+connectionFields+" FROM connections WHERE id=? AND user_id=?", id, userID))
}
func (r *Repository) UpdateConnection(ctx context.Context, c models.Connection) (models.Connection, error) {
	c.UpdatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx, `UPDATE connections SET name=?,driver=?,host=?,port=?,username=?,password_encrypted=?,initial_database=?,color=?,environment=?,ssl_enabled=?,timeout_seconds=?,updated_at=? WHERE id=? AND user_id=?`, c.Name, c.Driver, c.Host, c.Port, c.Username, c.PasswordEncrypted, c.InitialDatabase, c.Color, c.Environment, c.SSLEnabled, c.TimeoutSeconds, c.UpdatedAt, c.ID, c.UserID)
	if err != nil {
		return c, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return c, err
	}
	if n == 0 {
		return c, sql.ErrNoRows
	}
	return c, nil
}
func (r *Repository) DeleteConnection(ctx context.Context, userID, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"query_history", "saved_queries", "smart_queries"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+quoteIdentifier(table)+" WHERE user_id=? AND connection_id=?", userID, id); err != nil {
			return err
		}
	}
	res, err := tx.ExecContext(ctx, "DELETE FROM connections WHERE id=? AND user_id=?", id, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}
func (r *Repository) AddHistory(ctx context.Context, h models.QueryHistory) error {
	newID, err := id()
	if err != nil {
		return err
	}
	h.ID = newID
	h.UserID = LocalUserID
	h.CreatedAt = time.Now().UTC()
	_, err = r.db.ExecContext(ctx, `INSERT INTO query_history (id,user_id,connection_id,sql_text,operation_type,status,error_message,execution_time_ms,affected_rows,created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`, h.ID, h.UserID, h.ConnectionID, h.SQL, h.Type, h.Status, h.ErrorMessage, h.ExecutionTimeMs, h.AffectedRows, h.CreatedAt)
	return err
}
func (r *Repository) ListHistory(ctx context.Context, userID string, limit int) ([]models.QueryHistory, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,user_id,connection_id,sql_text,operation_type,status,error_message,execution_time_ms,affected_rows,created_at FROM query_history WHERE user_id=? ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.QueryHistory, 0)
	for rows.Next() {
		var h models.QueryHistory
		if err = rows.Scan(&h.ID, &h.UserID, &h.ConnectionID, &h.SQL, &h.Type, &h.Status, &h.ErrorMessage, &h.ExecutionTimeMs, &h.AffectedRows, &h.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
func (r *Repository) ConnectionStats(ctx context.Context, userID, connectionID string) (models.ConnectionStats, error) {
	stats := models.ConnectionStats{QueriesByDay: make([]models.QueryDayStat, 0, 7), QueriesByOperation: make([]models.QueryOperationStat, 0)}
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN status='success' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status='error' THEN 1 ELSE 0 END), 0),
		COALESCE(CAST(AVG(execution_time_ms) AS INTEGER), 0),
		COALESCE(SUM(affected_rows), 0)
		FROM query_history WHERE user_id=? AND connection_id=?`, userID, connectionID).
		Scan(&stats.TotalQueries, &stats.SuccessfulQueries, &stats.FailedQueries, &stats.AverageExecutionMs, &stats.AffectedRows)
	if err != nil {
		return stats, err
	}
	var lastQuery time.Time
	if err := r.db.QueryRowContext(ctx, `SELECT created_at FROM query_history WHERE user_id=? AND connection_id=? ORDER BY created_at DESC LIMIT 1`, userID, connectionID).Scan(&lastQuery); err == nil {
		stats.LastQueryAt = &lastQuery
	} else if err != sql.ErrNoRows {
		return stats, err
	}
	start := time.Now().UTC().AddDate(0, 0, -6).Truncate(24 * time.Hour)
	byDay := map[string]*models.QueryDayStat{}
	for i := 0; i < 7; i++ {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		item := models.QueryDayStat{Date: date}
		stats.QueriesByDay = append(stats.QueriesByDay, item)
		byDay[date] = &stats.QueriesByDay[len(stats.QueriesByDay)-1]
	}
	rows, err := r.db.QueryContext(ctx, `SELECT COALESCE(strftime('%Y-%m-%d', created_at), ''),
		SUM(CASE WHEN status='success' THEN 1 ELSE 0 END),
		SUM(CASE WHEN status='error' THEN 1 ELSE 0 END)
		FROM query_history WHERE user_id=? AND connection_id=? AND created_at>=?
		GROUP BY strftime('%Y-%m-%d', created_at)`, userID, connectionID, start)
	if err != nil {
		return stats, err
	}
	defer rows.Close()
	for rows.Next() {
		var date string
		var success, failed int
		if err := rows.Scan(&date, &success, &failed); err != nil {
			return stats, err
		}
		if item := byDay[date]; item != nil {
			item.Success, item.Failed = success, failed
		}
	}
	if err := rows.Err(); err != nil {
		return stats, err
	}
	rows, err = r.db.QueryContext(ctx, `SELECT operation_type, COUNT(*) FROM query_history
		WHERE user_id=? AND connection_id=? GROUP BY operation_type ORDER BY COUNT(*) DESC LIMIT 6`, userID, connectionID)
	if err != nil {
		return stats, err
	}
	defer rows.Close()
	for rows.Next() {
		var item models.QueryOperationStat
		if err := rows.Scan(&item.Operation, &item.Count); err != nil {
			return stats, err
		}
		stats.QueriesByOperation = append(stats.QueriesByOperation, item)
	}
	return stats, rows.Err()
}
func (r *Repository) ConnectionCount(ctx context.Context, userID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM connections WHERE user_id=?`, userID).Scan(&count)
	return count, err
}
func (r *Repository) DeleteHistory(ctx context.Context, userID, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM query_history WHERE id=? AND user_id=?", id, userID)
	return err
}
func (r *Repository) ListSavedQueries(ctx context.Context, userID string) ([]models.SavedQuery, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,user_id,connection_id,name,sql_text,updated_at FROM saved_queries WHERE user_id=? ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	queries := []models.SavedQuery{}
	for rows.Next() {
		var query models.SavedQuery
		if err := rows.Scan(&query.ID, &query.UserID, &query.ConnectionID, &query.Name, &query.SQL, &query.UpdatedAt); err != nil {
			return nil, err
		}
		queries = append(queries, query)
	}
	return queries, rows.Err()
}
func (r *Repository) SaveSavedQuery(ctx context.Context, query models.SavedQuery) (models.SavedQuery, error) {
	query.UserID = LocalUserID
	if query.ID == "" {
		var err error
		if query.ID, err = id(); err != nil {
			return query, err
		}
	}
	query.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `INSERT INTO saved_queries (id,user_id,connection_id,name,sql_text,updated_at) VALUES (?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET connection_id=excluded.connection_id,name=excluded.name,sql_text=excluded.sql_text,updated_at=excluded.updated_at`, query.ID, query.UserID, query.ConnectionID, query.Name, query.SQL, query.UpdatedAt)
	return query, err
}
func (r *Repository) DeleteSavedQuery(ctx context.Context, userID, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM saved_queries WHERE id=? AND user_id=?`, id, userID)
	return err
}
func (r *Repository) ListSmartQueries(ctx context.Context, userID string) ([]models.WorkspaceSmartQuery, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,user_id,connection_id,title,description,sql_text,source_sql,parameters_json,created_at FROM smart_queries WHERE user_id=? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	queries := []models.WorkspaceSmartQuery{}
	for rows.Next() {
		var query models.WorkspaceSmartQuery
		var parameters string
		if err := rows.Scan(&query.ID, &query.UserID, &query.ConnectionID, &query.Title, &query.Description, &query.SQL, &query.SourceSQL, &parameters, &query.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(parameters), &query.Parameters); err != nil {
			return nil, fmt.Errorf("decode smart query %s parameters: %w", query.ID, err)
		}
		queries = append(queries, query)
	}
	return queries, rows.Err()
}
func (r *Repository) SaveSmartQuery(ctx context.Context, query models.WorkspaceSmartQuery) (models.WorkspaceSmartQuery, error) {
	query.UserID = LocalUserID
	if query.ID == "" {
		var err error
		if query.ID, err = id(); err != nil {
			return query, err
		}
	}
	if query.CreatedAt.IsZero() {
		query.CreatedAt = time.Now().UTC()
	}
	parameters, err := json.Marshal(query.Parameters)
	if err != nil {
		return query, err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO smart_queries (id,user_id,connection_id,title,description,sql_text,source_sql,parameters_json,created_at) VALUES (?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET connection_id=excluded.connection_id,title=excluded.title,description=excluded.description,sql_text=excluded.sql_text,source_sql=excluded.source_sql,parameters_json=excluded.parameters_json`, query.ID, query.UserID, query.ConnectionID, query.Title, query.Description, query.SQL, query.SourceSQL, string(parameters), query.CreatedAt)
	return query, err
}
func (r *Repository) DeleteSmartQuery(ctx context.Context, userID, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM smart_queries WHERE id=? AND user_id=?`, id, userID)
	return err
}
func (r *Repository) GetAISetting(ctx context.Context, userID string) (models.AISetting, error) {
	var s models.AISetting
	err := r.db.QueryRowContext(ctx, `SELECT user_id,provider,model,base_url,api_key_encrypted,updated_at FROM ai_settings WHERE user_id=?`, userID).Scan(&s.UserID, &s.Provider, &s.Model, &s.BaseURL, &s.APIKeyEncrypted, &s.UpdatedAt)
	return s, err
}
func (r *Repository) SaveAISetting(ctx context.Context, s models.AISetting) error {
	s.UserID = LocalUserID
	s.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `INSERT INTO ai_settings (user_id,provider,model,base_url,api_key_encrypted,updated_at) VALUES (?,?,?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET provider=excluded.provider,model=excluded.model,base_url=excluded.base_url,api_key_encrypted=excluded.api_key_encrypted,updated_at=excluded.updated_at`, s.UserID, s.Provider, s.Model, s.BaseURL, s.APIKeyEncrypted, s.UpdatedAt)
	return err
}

func (r *Repository) GetBackupSetting(ctx context.Context, userID string) (models.BackupSetting, error) {
	var s models.BackupSetting
	err := r.db.QueryRowContext(ctx, `SELECT user_id,endpoint,bucket,region,access_key_encrypted,secret_encrypted,updated_at FROM backup_settings WHERE user_id=?`, userID).Scan(&s.UserID, &s.Endpoint, &s.Bucket, &s.Region, &s.AccessKeyEncrypted, &s.SecretEncrypted, &s.UpdatedAt)
	return s, err
}

func (r *Repository) SaveBackupSetting(ctx context.Context, s models.BackupSetting) error {
	s.UserID = LocalUserID
	s.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `INSERT INTO backup_settings (user_id,endpoint,bucket,region,access_key_encrypted,secret_encrypted,updated_at) VALUES (?,?,?,?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET endpoint=excluded.endpoint,bucket=excluded.bucket,region=excluded.region,access_key_encrypted=excluded.access_key_encrypted,secret_encrypted=excluded.secret_encrypted,updated_at=excluded.updated_at`, s.UserID, s.Endpoint, s.Bucket, s.Region, s.AccessKeyEncrypted, s.SecretEncrypted, s.UpdatedAt)
	return err
}

// DumpSQL serializes all workspace data as portable SQLite INSERT statements.
// Backup settings are deliberately excluded, so a restored backup cannot
// replace the S3 credentials currently being used to retrieve it.
func (r *Repository) DumpSQL(ctx context.Context) (string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT IN ('schema_migrations', 'backup_settings') ORDER BY name`)
	if err != nil {
		return "", err
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return "", err
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()

	var out strings.Builder
	out.WriteString("-- DBfock workspace backup\nBEGIN;\nPRAGMA defer_foreign_keys = ON;\n")
	for _, table := range tables {
		fmt.Fprintf(&out, "DELETE FROM %s;\n", quoteIdentifier(table))
	}
	for _, table := range tables {
		data, err := r.db.QueryContext(ctx, "SELECT * FROM "+quoteIdentifier(table))
		if err != nil {
			return "", err
		}
		columns, err := data.Columns()
		if err != nil {
			data.Close()
			return "", err
		}
		quotedColumns := make([]string, len(columns))
		for i, column := range columns {
			quotedColumns[i] = quoteIdentifier(column)
		}
		for data.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := data.Scan(pointers...); err != nil {
				data.Close()
				return "", err
			}
			encoded := make([]string, len(values))
			for i, value := range values {
				encoded[i] = quoteSQLValue(value)
			}
			fmt.Fprintf(&out, "INSERT INTO %s (%s) VALUES (%s);\n", quoteIdentifier(table), strings.Join(quotedColumns, ","), strings.Join(encoded, ","))
		}
		if err := data.Err(); err != nil {
			data.Close()
			return "", err
		}
		data.Close()
	}
	out.WriteString("COMMIT;\n")
	return out.String(), nil
}

func (r *Repository) RestoreSQL(ctx context.Context, script string) error {
	if !strings.HasPrefix(strings.TrimSpace(script), "-- DBfock workspace backup") || !strings.Contains(script, "BEGIN;") || !strings.Contains(script, "COMMIT;") {
		return fmt.Errorf("invalid DBfock backup file")
	}
	_, err := r.db.ExecContext(ctx, script)
	return err
}

func quoteIdentifier(value string) string { return `"` + strings.ReplaceAll(value, `"`, `""`) + `"` }
func quoteSQLValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "NULL"
	case []byte:
		return "X'" + fmt.Sprintf("%x", v) + "'"
	case time.Time:
		return "'" + strings.ReplaceAll(v.UTC().Format(time.RFC3339Nano), "'", "''") + "'"
	case bool:
		if v {
			return "1"
		}
		return "0"
	default:
		return "'" + strings.ReplaceAll(fmt.Sprint(v), "'", "''") + "'"
	}
}

func (r *Repository) AddAIAuditLog(ctx context.Context, log models.AIAuditLog) error {
	newID, err := id()
	if err != nil {
		return err
	}
	log.ID = newID
	log.CreatedAt = time.Now().UTC()
	_, err = r.db.ExecContext(ctx, `INSERT INTO ai_audit_logs (id,user_id,run_id,question,stage,provider,model,request_text,response_text,error_message,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, log.ID, LocalUserID, log.RunID, log.Question, log.Stage, log.Provider, log.Model, log.Request, log.Response, log.Error, log.CreatedAt)
	return err
}

func (r *Repository) ListAIAuditLogs(ctx context.Context, userID string, limit int) ([]models.AIAuditLog, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,run_id,question,stage,provider,model,request_text,response_text,error_message,created_at FROM ai_audit_logs WHERE user_id=? ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs := []models.AIAuditLog{}
	for rows.Next() {
		var log models.AIAuditLog
		if err := rows.Scan(&log.ID, &log.RunID, &log.Question, &log.Stage, &log.Provider, &log.Model, &log.Request, &log.Response, &log.Error, &log.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *Repository) CreateAIChatJob(ctx context.Context) (models.AIChatJob, error) {
	job := models.AIChatJob{Status: "running", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	var err error
	if job.ID, err = id(); err != nil {
		return job, err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO ai_chat_jobs (id,user_id,status,created_at,updated_at) VALUES (?,?,?,?,?)`, job.ID, LocalUserID, job.Status, job.CreatedAt, job.UpdatedAt)
	return job, err
}

func (r *Repository) GetAIChatJob(ctx context.Context, userID, id string) (models.AIChatJob, error) {
	var job models.AIChatJob
	err := r.db.QueryRowContext(ctx, `SELECT id,status,message,error_message,created_at,updated_at FROM ai_chat_jobs WHERE id=? AND user_id=?`, id, userID).Scan(&job.ID, &job.Status, &job.Message, &job.Error, &job.CreatedAt, &job.UpdatedAt)
	return job, err
}

func (r *Repository) CompleteAIChatJob(ctx context.Context, id, message string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ai_chat_jobs SET status='complete',message=?,error_message='',updated_at=? WHERE id=? AND user_id=?`, message, time.Now().UTC(), id, LocalUserID)
	return err
}

func (r *Repository) FailAIChatJob(ctx context.Context, id, message string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ai_chat_jobs SET status='failed',error_message=?,updated_at=? WHERE id=? AND user_id=?`, message, time.Now().UTC(), id, LocalUserID)
	return err
}
