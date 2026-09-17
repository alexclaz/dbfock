package mysql

import (
	"testing"

	"github.com/dbfock/database-manager/backend/internal/models"
)

func TestColumnsEqualChecksStructuralProperties(t *testing.T) {
	defaultValue := "0"
	base := schemaColumn{name: "active", columnType: "tinyint(1)", nullable: false, defaultValue: &defaultValue, key: "MUL", extra: ""}
	if !columnsEqual(base, base) {
		t.Fatal("identical columns were reported as different")
	}

	different := base
	different.nullable = true
	if columnsEqual(base, different) {
		t.Fatal("nullable difference was ignored")
	}
	different = base
	different.columnType = "int"
	if columnsEqual(base, different) {
		t.Fatal("type difference was ignored")
	}
	different = base
	different.key = ""
	if columnsEqual(base, different) {
		t.Fatal("index-key difference was ignored")
	}
}

func TestEstimatedValuesSizeAccountsForLargeValues(t *testing.T) {
	small := estimatedValuesSize([]any{"hello", int64(1)})
	large := estimatedValuesSize([]any{make([]byte, insertBatchBytes)})
	if small <= 0 {
		t.Fatalf("small size = %d, want positive", small)
	}
	if large <= insertBatchBytes {
		t.Fatalf("large size = %d, want greater than batch threshold", large)
	}
}

func TestMigrationValueUsesTextCharsetForJSON(t *testing.T) {
	jsonValue := migrationValue([]byte(`{"pet":"Fubá"}`), "json")
	if _, ok := jsonValue.(string); !ok {
		t.Fatalf("JSON value type = %T, want string", jsonValue)
	}
	binaryValue := migrationValue([]byte{0, 1, 2}, "longblob")
	if _, ok := binaryValue.([]byte); !ok {
		t.Fatalf("BLOB value type = %T, want []byte", binaryValue)
	}
}

func TestMigrationSQLModeAllowsLegacyZeroDates(t *testing.T) {
	got := migrationSQLMode("STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO")
	want := "STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO"
	if got != want {
		t.Fatalf("migrationSQLMode() = %q, want %q", got, want)
	}
}

func TestMappedDatabaseMigrationOptions(t *testing.T) {
	options := models.DatabaseMigrationOptions{Databases: []string{"source_db"}, TargetDatabase: "target_db", MaxTableSizeBytes: 1, Strategy: "merge"}
	if err := validateMigrationOptions(options); err != nil {
		t.Fatalf("validateMigrationOptions() error = %v", err)
	}
	if got := migrationTargetDatabase(options, "source_db"); got != "target_db" {
		t.Fatalf("migrationTargetDatabase() = %q, want target_db", got)
	}
	options.Databases = append(options.Databases, "other_db")
	if err := validateMigrationOptions(options); err == nil {
		t.Fatal("mapped migration with multiple source databases was accepted")
	}
}

func TestMigrationTableMatchesSizeAndEstimatedRows(t *testing.T) {
	source := migrationTable{name: "pets", sizeBytes: 1024, estimatedRows: 10}
	if !migrationTableMatches(source, migrationTable{name: "pets", sizeBytes: 1024, estimatedRows: 10}) {
		t.Fatal("equal table metadata was not matched")
	}
	if migrationTableMatches(source, migrationTable{name: "pets", sizeBytes: 1024, estimatedRows: 11}) {
		t.Fatal("different estimated row counts were matched")
	}
	if migrationTableMatches(source, migrationTable{name: "pets", sizeBytes: 2048, estimatedRows: 10}) {
		t.Fatal("different data sizes were matched")
	}
}

func TestSelectedMigrationTablesUsesDatabaseAndTable(t *testing.T) {
	selected := selectedMigrationTables(models.DatabaseMigrationOptions{SelectedTables: []models.DatabaseMigrationSelection{{Database: "shop", Table: "pets"}}})
	if !selected[migrationSelectionKey("shop", "pets")] {
		t.Fatal("selected table was not indexed")
	}
	if selected[migrationSelectionKey("archive", "pets")] {
		t.Fatal("a table from another database was selected")
	}
	if selectedMigrationTables(models.DatabaseMigrationOptions{}) != nil {
		t.Fatal("empty selection should preserve the default all-tables behavior")
	}
}

func TestUnionKeysIsSortedAndUnique(t *testing.T) {
	keys := unionKeys(map[string]int{"b": 1, "a": 1}, map[string]int{"c": 1, "a": 2})
	want := []string{"a", "b", "c"}
	for index := range want {
		if keys[index] != want[index] {
			t.Fatalf("keys = %#v, want %#v", keys, want)
		}
	}
}
