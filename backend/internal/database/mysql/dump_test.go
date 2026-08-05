package mysql

import (
	"testing"
	"time"
)

func TestSQLLiteralEscapesValuesTheServerCanReadBack(t *testing.T) {
	stamp := time.Date(2026, 8, 4, 15, 30, 45, 0, time.UTC)
	cases := []struct {
		name         string
		value        any
		databaseType string
		want         string
	}{
		{name: "null", value: nil, want: "NULL"},
		{name: "integer", value: int64(-42), want: "-42"},
		{name: "boolean", value: true, want: "1"},
		{name: "datetime", value: stamp, databaseType: "DATETIME", want: "'2026-08-04 15:30:45'"},
		{name: "text", value: []byte("plain"), databaseType: "VARCHAR", want: "'plain'"},
		{name: "quote", value: []byte("O'Brien"), databaseType: "VARCHAR", want: `'O\'Brien'`},
		{name: "backslash", value: []byte(`C:\tmp`), databaseType: "VARCHAR", want: `'C:\\tmp'`},
		{name: "newline", value: []byte("a\nb"), databaseType: "TEXT", want: `'a\nb'`},
		{name: "blob", value: []byte{0x00, 0xff}, databaseType: "BLOB", want: "0x00ff"},
		{name: "empty blob", value: []byte{}, databaseType: "BLOB", want: "''"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := sqlLiteral(item.value, item.databaseType); got != item.want {
				t.Fatalf("sqlLiteral() = %s, want %s", got, item.want)
			}
		})
	}
}

// A string column holding binary-looking bytes must stay a quoted string, so a
// dump of text data never turns into hex literals.
func TestSQLLiteralKeepsTextColumnsQuoted(t *testing.T) {
	if got := sqlLiteral([]byte("0x01"), "VARCHAR"); got != "'0x01'" {
		t.Fatalf("sqlLiteral() = %s, want '0x01'", got)
	}
}
