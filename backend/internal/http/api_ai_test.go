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

func TestAIDatabaseSelectionCanSkipLiveSchemaForGeneralSQLHelp(t *testing.T) {
	var selection aiDatabaseSelection
	if err := json.Unmarshal([]byte(`{"needsSchema":false,"databases":[]}`), &selection); err != nil {
		t.Fatalf("decode selection: %v", err)
	}
	if !canAnswerWithoutAISchema(selection) {
		t.Fatal("a schema-independent request should skip live schema discovery")
	}
	if canAnswerWithoutAISchema(aiDatabaseSelection{}) {
		t.Fatal("an incomplete selection must retain the safe schema workflow")
	}
}

func TestPartialAIGenerationShowsAnswerBeforeStreamCompletes(t *testing.T) {
	partial := partialAIGeneration(`{"answer":"A query uses`)
	if partial != "A query uses" {
		t.Fatalf("partial answer = %q", partial)
	}
	withSQL := partialAIGeneration(`{"answer":"Use an index.","sql":"SELECT * FROM clientes"`)
	if withSQL != "Use an index.\n\n```sql\nSELECT * FROM clientes" {
		t.Fatalf("partial SQL response = %q", withSQL)
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

func TestFastSchemaTablesUsesLocalNamesAndForeignKeys(t *testing.T) {
	cliente := aiTableRef{Database: "crm", Table: "clientes"}
	fatura := aiTableRef{Database: "crm", Table: "faturas"}
	auditoria := aiTableRef{Database: "crm", Table: "auditoria"}
	catalog := cachedAISchema{
		tables: []aiTableRef{auditoria, cliente, fatura},
		structures: map[aiTableRef]*models.TableStructure{
			cliente:   {Columns: []models.ColumnInfo{{Name: "id"}, {Name: "nome"}}},
			fatura:    {Columns: []models.ColumnInfo{{Name: "id"}, {Name: "cliente_id"}, {Name: "valor"}}, ForeignKeys: []models.ForeignKeyInfo{{Column: "cliente_id", ReferencedTable: "clientes", ReferencedColumn: "id"}}},
			auditoria: {Columns: []models.ColumnInfo{{Name: "evento"}}},
		},
	}

	selected := fastSchemaTables(catalog, aiChatRequest{Prompt: "liste as faturas por cliente"}, "crm")
	if !containsAITable(selected, fatura) || !containsAITable(selected, cliente) {
		t.Fatalf("fast retrieval did not include the matching table and join target: %#v", selected)
	}
	if containsAITable(selected, auditoria) {
		t.Fatalf("fast retrieval included unrelated table: %#v", selected)
	}
}

func TestFastSchemaTablesRespectsManualTableScope(t *testing.T) {
	first := aiTableRef{Database: "crm", Table: "clientes"}
	second := aiTableRef{Database: "crm", Table: "faturas"}
	catalog := cachedAISchema{tables: []aiTableRef{first, second}, structures: map[aiTableRef]*models.TableStructure{first: {Columns: []models.ColumnInfo{{Name: "id"}}}, second: {Columns: []models.ColumnInfo{{Name: "id"}}}}}
	selected := fastSchemaTables(catalog, aiChatRequest{TableScope: "selected", SelectedTables: []aiTableRef{second}}, "crm")
	if len(selected) != 1 || selected[0] != second {
		t.Fatalf("manual table scope was not preserved: %#v", selected)
	}
}

func TestValidateFastGeneratedSQLRejectsUnknownQuotedIdentifier(t *testing.T) {
	table := aiTableRef{Database: "crm", Table: "clientes"}
	structures := map[aiTableRef]*models.TableStructure{table: {Columns: []models.ColumnInfo{{Name: "id"}, {Name: "nome"}}}}
	if err := validateFastGeneratedSQL("SELECT `nome` FROM `crm`.`clientes`", []aiTableRef{table}, structures); err != nil {
		t.Fatalf("known identifiers were rejected: %v", err)
	}
	if err := validateFastGeneratedSQL("SELECT `email` FROM `crm`.`clientes`", []aiTableRef{table}, structures); err == nil {
		t.Fatal("unknown identifier was accepted")
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
