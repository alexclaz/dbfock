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

func TestMigrationSQLModeAcceptsLegacySourceData(t *testing.T) {
	got := migrationSQLMode("STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO")
	want := "ERROR_FOR_DIVISION_BY_ZERO"
	if got != want {
		t.Fatalf("migrationSQLMode() = %q, want %q", got, want)
	}
}

// A float(12,4) clamped under a non-strict server reads back as 100000000, above
// its own column maximum of 99999999.9999. Keeping strict mode fails the insert
// and rolls back the whole table over a row the source considers valid.
func TestMigrationSQLModeDropsEveryStrictMode(t *testing.T) {
	got := migrationSQLMode("ONLY_FULL_GROUP_BY,STRICT_ALL_TABLES,strict_trans_tables,NO_ENGINE_SUBSTITUTION")
	want := "ONLY_FULL_GROUP_BY,NO_ENGINE_SUBSTITUTION"
	if got != want {
		t.Fatalf("migrationSQLMode() = %q, want %q", got, want)
	}
}

func TestMigrationSQLModeKeepsUnrelatedModesUntouched(t *testing.T) {
	const modes = "ONLY_FULL_GROUP_BY,NO_ENGINE_SUBSTITUTION"
	if got := migrationSQLMode(modes); got != modes {
		t.Fatalf("migrationSQLMode() = %q, want %q", got, modes)
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
		t.Fatal("omitted selection should preserve the default all-tables behavior")
	}
	if selected := selectedMigrationTables(models.DatabaseMigrationOptions{SelectedTables: []models.DatabaseMigrationSelection{}}); selected == nil || len(selected) != 0 {
		t.Fatal("explicit empty selection should select no tables")
	}
}

func TestStructureOnlyMigrationTablesUsesExplicitSelection(t *testing.T) {
	options := models.DatabaseMigrationOptions{CreateSkippedStructures: true, StructureOnlyTables: []models.DatabaseMigrationSelection{{Database: "shop", Table: "audit-log"}}}
	selected := structureOnlyMigrationTables(options)
	if !selected[migrationSelectionKey("shop", "audit-log")] {
		t.Fatal("structure-only table was not indexed")
	}
	if selected[migrationSelectionKey("shop", "pets")] {
		t.Fatal("unselected table was included in structure-only selection")
	}
	options.CreateSkippedStructures = false
	if selected := structureOnlyMigrationTables(options); len(selected) != 0 {
		t.Fatal("structure-only selection was used while its option was disabled")
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
