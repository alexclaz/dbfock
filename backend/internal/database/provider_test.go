package database

import (
	"strings"
	"testing"
)

func TestQuoteIdentifierSupportsMySQLQuotedNames(t *testing.T) {
	tests := map[string]string{
		"2024_dados":      "`2024_dados`",
		"pedido-item":     "`pedido-item`",
		"nome com espaço": "`nome com espaço`",
		"preço":           "`preço`",
		"com`crase":       "`com``crase`",
	}
	for input, want := range tests {
		got, err := QuoteIdentifier(input)
		if err != nil {
			t.Fatalf("QuoteIdentifier(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("QuoteIdentifier(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestValidateIdentifierRejectsUnsafeOrUnsupportedNames(t *testing.T) {
	for _, input := range []string{"", "nul\x00byte", strings.Repeat("a", 65)} {
		if err := ValidateIdentifier(input); err == nil {
			t.Fatalf("ValidateIdentifier(%q) accepted invalid name", input)
		}
	}
}
