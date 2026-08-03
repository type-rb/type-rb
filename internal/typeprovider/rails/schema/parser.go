package schema

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/token"
)

func Parse(source []byte) (*Schema, error) {
	tokens, diagnostics := lexer.Lex(source)
	if len(diagnostics) > 0 {
		return nil, fmt.Errorf("schema.rb:%s", diagnostics[0])
	}
	result := &Schema{}
	var current *Table
	for _, line := range lines(tokens) {
		if len(line) == 0 {
			continue
		}
		if current == nil {
			if line[0].Lexeme != "create_table" {
				continue
			}
			table, err := parseTable(line)
			if err != nil {
				return nil, err
			}
			result.Tables = append(result.Tables, table)
			current = &result.Tables[len(result.Tables)-1]
			continue
		}
		if len(line) == 1 && line[0].Lexeme == "end" {
			current.Span.End = line[0].Span.End
			current = nil
			continue
		}
		column, ok, err := parseColumn(line)
		if err != nil {
			return nil, err
		}
		if ok {
			current.Columns = append(current.Columns, column...)
		}
	}
	if current != nil {
		return nil, fmt.Errorf("schema.rb:%d:%d: create_table %s is missing end", current.Span.Start.Line, current.Span.Start.Column, current.Name)
	}
	return result, nil
}

func parseTable(line []token.Token) (Table, error) {
	if len(line) < 2 || line[1].Kind != token.String {
		return Table{}, at(line[0], "create_table requires a string table name")
	}
	name, err := stringValue(line[1])
	if err != nil {
		return Table{}, err
	}
	result := Table{Name: name, ID: true, Span: lineSpan(line)}
	if value, ok := keywordValue(line, "id"); ok && value == "false" {
		result.ID = false
	}
	return result, nil
}

func parseColumn(line []token.Token) ([]Column, bool, error) {
	if len(line) < 3 || line[0].Lexeme != "t" || line[1].Lexeme != "." {
		return nil, false, nil
	}
	method := line[2].Lexeme
	if method == "index" || method == "foreign_key" || method == "check_constraint" {
		return nil, false, nil
	}
	if method == "timestamps" {
		nullable := nullableOption(line)
		span := lineSpan(line)
		return []Column{{Name: "created_at", DatabaseType: "datetime", Nullable: nullable, Span: span}, {Name: "updated_at", DatabaseType: "datetime", Nullable: nullable, Span: span}}, true, nil
	}
	if len(line) < 4 || line[3].Kind != token.String {
		return nil, false, nil
	}
	name, err := stringValue(line[3])
	if err != nil {
		return nil, false, err
	}
	databaseType := method
	if method == "column" {
		if len(line) < 6 || line[5].Kind != token.String {
			return nil, false, at(line[2], "t.column requires a name and database type")
		}
		databaseType, err = stringValue(line[5])
		if err != nil {
			return nil, false, err
		}
	}
	return []Column{{Name: name, DatabaseType: databaseType, Nullable: nullableOption(line), Span: lineSpan(line)}}, true, nil
}

func nullableOption(line []token.Token) bool {
	value, ok := keywordValue(line, "null")
	return !ok || value != "false"
}

func keywordValue(line []token.Token, name string) (string, bool) {
	for index := 0; index+2 < len(line); index++ {
		if line[index].Lexeme == name && line[index+1].Lexeme == ":" {
			return line[index+2].Lexeme, true
		}
	}
	return "", false
}

func stringValue(value token.Token) (string, error) {
	if value.Kind != token.String || len(value.Lexeme) < 2 {
		return "", at(value, "expected string literal")
	}
	if value.Lexeme[0] == '\'' {
		return strings.ReplaceAll(value.Lexeme[1:len(value.Lexeme)-1], `\'`, `'`), nil
	}
	decoded, err := strconv.Unquote(value.Lexeme)
	if err != nil {
		return "", at(value, "invalid string literal")
	}
	return decoded, nil
}

func lines(tokens []token.Token) [][]token.Token {
	var result [][]token.Token
	var current []token.Token
	for _, item := range tokens {
		switch item.Kind {
		case token.Newline, token.EOF:
			if len(current) > 0 {
				result = append(result, current)
				current = nil
			}
		case token.Comment:
			// Comments never carry schema type information.
		default:
			current = append(current, item)
		}
	}
	return result
}

func lineSpan(line []token.Token) token.Span {
	return token.Span{Start: line[0].Span.Start, End: line[len(line)-1].Span.End}
}

func at(item token.Token, message string) error {
	return fmt.Errorf("schema.rb:%d:%d: %s", item.Span.Start.Line, item.Span.Start.Column, message)
}
