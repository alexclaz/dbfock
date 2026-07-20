package httpapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dbfock/database-manager/backend/internal/models"
)

func TestScopeConfirmationMessageCarriesOnlySuggestedIdentifiers(t *testing.T) {
	message := scopeConfirmationMessage("tables", "liste produtos", []string{"catalogo"}, []aiTableRef{{Database: "catalogo", Table: "produtos"}})
	if !strings.HasPrefix(message, scopeConfirmationPrefix) {
		t.Fatalf("scope confirmation prefix missing: %q", message)
	}
	var confirmation aiScopeConfirmation
	if err := json.Unmarshal([]byte(strings.TrimPrefix(message, scopeConfirmationPrefix)), &confirmation); err != nil {
		t.Fatalf("decode scope confirmation: %v", err)
	}
	if confirmation.Step != "tables" || confirmation.Prompt != "liste produtos" || len(confirmation.Databases) != 1 || confirmation.Databases[0] != "catalogo" || len(confirmation.Tables) != 1 || confirmation.Tables[0].Table != "produtos" {
		t.Fatalf("unexpected scope confirmation: %#v", confirmation)
	}
}

func TestProgressiveSelectionKeepsOnlyChosenSchema(t *testing.T) {
	tables := []aiTableRef{{Database: "petshop", Table: "animais"}, {Database: "petshop", Table: "empresas"}, {Database: "petshop", Table: "vendas"}}
	structures := map[aiTableRef]*models.TableStructure{
		tables[0]: {Columns: []models.ColumnInfo{{Name: "id", ColumnType: "bigint"}, {Name: "nome", ColumnType: "varchar(50)"}, {Name: "empresa_id", ColumnType: "bigint"}}, ForeignKeys: []models.ForeignKeyInfo{{Column: "empresa_id", ReferencedTable: "empresas", ReferencedColumn: "id"}}},
		tables[1]: {Columns: []models.ColumnInfo{{Name: "id", ColumnType: "bigint"}, {Name: "nome", ColumnType: "varchar(50)"}}},
		tables[2]: {Columns: []models.ColumnInfo{{Name: "id", ColumnType: "bigint"}, {Name: "valor", ColumnType: "decimal(10,2)"}}},
	}

	chosenTables := selectedTables(aiTableSelection{Tables: tables[:2]}, tables, "quero animais e empresa")
	columns := selectedColumns(aiColumnSelection{Columns: []aiTableColumns{{Database: "petshop", Table: "animais", Columns: []string{"nome"}}, {Database: "petshop", Table: "empresas", Columns: []string{"nome"}}}}, chosenTables, structures)
	relationships := selectedRelationships(aiRelationshipSelection{Relationships: availableRelationships(chosenTables, structures)}, availableRelationships(chosenTables, structures))
	schema := aiColumnList(chosenTables, structures, columns, relationships)

	for _, expected := range []string{"`petshop`.`animais` (`nome` varchar(50), `empresa_id` bigint)", "`petshop`.`empresas` (`id` bigint, `nome` varchar(50))", "`petshop`.`animais`.`empresa_id` → `petshop`.`empresas`.`id`"} {
		if !strings.Contains(schema, expected) {
			t.Errorf("focused schema is missing %q:\n%s", expected, schema)
		}
	}
	if strings.Contains(schema, "vendas") || strings.Contains(schema, "`animais` (`id`") {
		t.Fatalf("focused schema leaked an unselected table or column:\n%s", schema)
	}
}

func TestProgressiveSelectionFallsBackToSelectedDatabaseAndPromptTable(t *testing.T) {
	databases := selectedDatabases(aiDatabaseSelection{Databases: []string{"nao-existe"}}, []string{"geral", "petshop"}, "petshop")
	if len(databases) != 1 || databases[0] != "petshop" {
		t.Fatalf("unexpected database fallback: %#v", databases)
	}
	tables := []aiTableRef{{Database: "petshop", Table: "animais"}, {Database: "petshop", Table: "empresas"}}
	chosen := selectedTables(aiTableSelection{}, tables, "liste os animais")
	if len(chosen) != 1 || chosen[0].Table != "animais" {
		t.Fatalf("unexpected table fallback: %#v", chosen)
	}
}

func TestManualAIScopeOnlyKeepsAccessibleDatabaseTables(t *testing.T) {
	databases := requestedDatabases([]string{"catalogo", "CATALOGO", "ausente"}, []string{"geral", "catalogo"})
	if len(databases) != 1 || databases[0] != "catalogo" {
		t.Fatalf("unexpected manual databases: %#v", databases)
	}
	available := []aiTableRef{{Database: "catalogo", Table: "produtos"}, {Database: "catalogo", Table: "categorias"}}
	tables := requestedTables([]aiTableRef{{Database: "CATALOGO", Table: "PRODUTOS"}, {Database: "catalogo", Table: "ausente"}}, available)
	if len(tables) != 1 || tables[0] != available[0] {
		t.Fatalf("unexpected manual tables: %#v", tables)
	}
}

func TestAIConversationKeepsTheCurrentTabsPriorMessages(t *testing.T) {
	prompt := aiConversation([]aiChatMessage{
		{Role: "user", Content: "Liste os clientes ativos"},
		{Role: "assistant", Content: "```sql\nSELECT * FROM clientes WHERE ativo = 1\n```"},
		{Role: "user", Content: "Agora apenas os dez mais recentes"},
	}, "Agora apenas os dez mais recentes")

	for _, expected := range []string{
		"User: Liste os clientes ativos",
		"Assistant: ```sql",
		"Current user request: Agora apenas os dez mais recentes",
	} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("conversation is missing %q:\n%s", expected, prompt)
		}
	}
	if strings.Count(prompt, "Agora apenas os dez mais recentes") != 1 {
		t.Fatalf("current request was included more than once:\n%s", prompt)
	}
}

func TestSmartQueryRecognizesINFilter(t *testing.T) {
	sql := "SELECT *\nFROM geral.EMPRESA_GRL eg\nWHERE eg.ID_EMPRESA_EPGL in (16767)"
	if !smartQueryWherePattern.MatchString(sql) {
		t.Fatal("WHERE filter was not recognized")
	}
	if !validSmartQuery(models.SmartQuery{
		Title: "Empresa por ID", Description: "Consulta empresas pelo identificador.",
		SQL:        "SELECT * FROM `geral`.`EMPRESA_GRL` WHERE `ID_EMPRESA_EPGL` IN (:empresa_id)",
		Parameters: []models.SmartQueryParam{{Key: "empresa_id", DefaultValue: "16767"}},
	}) {
		t.Fatal("valid smart query for an IN filter was rejected")
	}
}
