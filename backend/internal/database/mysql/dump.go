package mysql

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dbfock/database-manager/backend/internal/database"
	"github.com/dbfock/database-manager/backend/internal/models"
)

// dumpFooter closes every dump. Callers rely on it to tell a complete dump from
// one cut short by an error, which cannot be signalled through the status code
// once streaming has begun.
const dumpFooter = "SET FOREIGN_KEY_CHECKS=1;\n"

// insertBatchBytes caps how much a single INSERT may grow, well below the
// server's default max_allowed_packet.
const insertBatchBytes = 500_000

// DumpDatabase streams a SQL dump of one database. Building it here rather than
// in the client turns a request per table page into a single response, and the
// rows never have to survive a JSON round trip on the way out.
func (p *Provider) DumpDatabase(ctx context.Context, c models.Connection, databaseName string, structureOnly bool, w io.Writer) error {
	quotedDatabase, err := database.QuoteIdentifier(databaseName)
	if err != nil {
		return err
	}
	tables, err := p.ListTables(ctx, c, databaseName, false)
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		return fmt.Errorf("database %s has no tables to dump", databaseName)
	}

	buffered := bufio.NewWriterSize(w, 64<<10)
	return p.withDB(c, func(db *sql.DB) error {
		if _, err := fmt.Fprintf(buffered, "SET FOREIGN_KEY_CHECKS=0;\nCREATE DATABASE IF NOT EXISTS %s;\nUSE %s;\n", quotedDatabase, quotedDatabase); err != nil {
			return err
		}
		for _, table := range tables {
			quotedTable, err := database.QuoteIdentifier(table.Name)
			if err != nil {
				return err
			}
			if err = writeTableDDL(ctx, db, buffered, quotedDatabase, quotedTable); err != nil {
				return err
			}
			if structureOnly {
				continue
			}
			if err = writeTableRows(ctx, db, buffered, quotedDatabase, quotedTable); err != nil {
				return err
			}
		}
		if _, err := buffered.WriteString(dumpFooter); err != nil {
			return err
		}
		return buffered.Flush()
	})
}

func writeTableDDL(ctx context.Context, db *sql.DB, w *bufio.Writer, quotedDatabase, quotedTable string) error {
	var name, ddl string
	if err := db.QueryRowContext(ctx, "SHOW CREATE TABLE "+quotedDatabase+"."+quotedTable).Scan(&name, &ddl); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "\nDROP TABLE IF EXISTS %s;\n%s;\n", quotedTable, strings.TrimSpace(ddl))
	return err
}

func writeTableRows(ctx context.Context, db *sql.DB, w *bufio.Writer, quotedDatabase, quotedTable string) error {
	rows, err := db.QueryContext(ctx, "SELECT * FROM "+quotedDatabase+"."+quotedTable)
	if err != nil {
		return err
	}
	defer rows.Close()

	types, err := rows.ColumnTypes()
	if err != nil {
		return err
	}
	columns := make([]string, len(types))
	for i, t := range types {
		quoted, err := database.QuoteIdentifier(t.Name())
		if err != nil {
			return err
		}
		columns[i] = quoted
	}
	prefix := fmt.Sprintf("INSERT INTO %s (%s) VALUES ", quotedTable, strings.Join(columns, ", "))

	values := make([]any, len(types))
	pointers := make([]any, len(types))
	for i := range values {
		pointers[i] = &values[i]
	}
	var batch strings.Builder
	for rows.Next() {
		if err = rows.Scan(pointers...); err != nil {
			return err
		}
		var row strings.Builder
		row.WriteByte('(')
		for i, value := range values {
			if i > 0 {
				row.WriteString(", ")
			}
			row.WriteString(sqlLiteral(value, types[i].DatabaseTypeName()))
		}
		row.WriteByte(')')

		if batch.Len() > 0 && len(prefix)+batch.Len()+row.Len()+2 > insertBatchBytes {
			if _, err = fmt.Fprintf(w, "%s%s;\n", prefix, batch.String()); err != nil {
				return err
			}
			batch.Reset()
		}
		if batch.Len() > 0 {
			batch.WriteString(", ")
		}
		batch.WriteString(row.String())
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if batch.Len() > 0 {
		if _, err = fmt.Fprintf(w, "%s%s;\n", prefix, batch.String()); err != nil {
			return err
		}
	}
	return nil
}

// sqlLiteral renders a scanned value the way the server will read it back.
// Binary columns become hex literals, because their bytes are not necessarily
// valid text in the dump's charset.
func sqlLiteral(value any, databaseType string) string {
	switch v := value.(type) {
	case nil:
		return "NULL"
	case bool:
		if v {
			return "1"
		}
		return "0"
	case int64:
		return fmt.Sprintf("%d", v)
	case uint64:
		return fmt.Sprintf("%d", v)
	case float32:
		return fmt.Sprintf("%v", v)
	case float64:
		return fmt.Sprintf("%v", v)
	case time.Time:
		return "'" + v.Format("2006-01-02 15:04:05.999999") + "'"
	case []byte:
		if isBinaryType(databaseType) {
			if len(v) == 0 {
				return "''"
			}
			return "0x" + hex.EncodeToString(v)
		}
		return quoteSQLString(string(v))
	case string:
		return quoteSQLString(v)
	default:
		return quoteSQLString(fmt.Sprintf("%v", v))
	}
}

func isBinaryType(databaseType string) bool {
	switch strings.ToUpper(databaseType) {
	case "BINARY", "VARBINARY", "BLOB", "TINYBLOB", "MEDIUMBLOB", "LONGBLOB", "BIT", "GEOMETRY":
		return true
	}
	return false
}

func quoteSQLString(value string) string {
	var out strings.Builder
	out.Grow(len(value) + 2)
	out.WriteByte('\'')
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\'':
			out.WriteString("\\'")
		case '\\':
			out.WriteString("\\\\")
		case 0:
			out.WriteString("\\0")
		case '\n':
			out.WriteString("\\n")
		case '\r':
			out.WriteString("\\r")
		case 0x1a:
			out.WriteString("\\Z")
		default:
			out.WriteByte(value[i])
		}
	}
	out.WriteByte('\'')
	return out.String()
}
