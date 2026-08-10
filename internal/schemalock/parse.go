package schemalock

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var createTablePattern = regexp.MustCompile(`(?is)^\s*CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?`)
var createUniqueIndexPattern = regexp.MustCompile(`(?is)^\s*CREATE\s+UNIQUE\s+(?:INDEX|KEY)\s+`)
var alterTablePattern = regexp.MustCompile(`(?is)^\s*ALTER\s+TABLE\s+(?:ONLY\s+)?`)

func ParseSQL(adapter string, source []byte) (*Lock, error) {
	lock := New(adapter)
	if err := lock.ValidateAdapter(); err != nil {
		return nil, err
	}
	cleaned := stripSQLComments(string(source))
	for _, statement := range splitSQL(cleaned, ';') {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		switch {
		case createTablePattern.MatchString(statement):
			name, table, err := parseCreateTable(adapter, statement)
			if err != nil {
				return nil, err
			}
			if _, exists := lock.Tables[name]; exists {
				return nil, fmt.Errorf("schema contains duplicate table %s", name)
			}
			lock.Tables[name] = table
		case createUniqueIndexPattern.MatchString(statement):
			if err := parseUniqueIndex(statement, lock); err != nil {
				return nil, err
			}
		case alterTablePattern.MatchString(statement):
			if err := parseAlterTable(statement, lock); err != nil {
				return nil, err
			}
		}
	}
	if len(lock.Tables) == 0 {
		return nil, errorsForSchema("schema contains no CREATE TABLE statements")
	}
	if err := lock.Validate(); err != nil {
		return nil, err
	}
	return lock, nil
}

func parseAlterTable(statement string, lock *Lock) error {
	rest := alterTablePattern.ReplaceAllString(statement, "")
	add := keywordIndex(rest, "ADD")
	if add < 0 {
		return nil
	}
	tableName := normalizeIdentifier(strings.TrimSpace(rest[:add]))
	table, exists := lock.Tables[tableName]
	if !exists {
		return fmt.Errorf("ALTER TABLE references unknown table %s", tableName)
	}
	definition := strings.TrimSpace(rest[add+len("ADD"):])
	if err := applyTableConstraint(tableName, definition, &table); err != nil {
		return err
	}
	lock.Tables[tableName] = table
	return nil
}

func (l *Lock) ValidateAdapter() error {
	switch l.Adapter {
	case "sqlite", "postgresql", "mysql":
		return nil
	default:
		return fmt.Errorf("unsupported schema adapter %q", l.Adapter)
	}
}

type schemaParseError string

func (e schemaParseError) Error() string { return string(e) }

func errorsForSchema(message string) error { return schemaParseError(message) }

func parseCreateTable(adapter, statement string) (string, Table, error) {
	rest := createTablePattern.ReplaceAllString(statement, "")
	open := topLevelIndex(rest, '(')
	if open < 0 {
		return "", Table{}, errorsForSchema("CREATE TABLE is missing a column list")
	}
	name := normalizeIdentifier(strings.TrimSpace(rest[:open]))
	if name == "" {
		return "", Table{}, errorsForSchema("CREATE TABLE is missing a table name")
	}
	close := matchingClose(rest, open)
	if close < 0 {
		return "", Table{}, fmt.Errorf("table %s is missing )", name)
	}
	table := Table{Columns: map[string]Column{}, ForeignKeys: map[string]ForeignKey{}, UniqueConstraints: map[string]UniqueConstraint{}}
	var constraints []string
	for _, item := range splitSQL(rest[open+1:close], ',') {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if tableConstraint(item) {
			constraints = append(constraints, item)
			continue
		}
		columnName, definition := leadingIdentifier(item)
		if columnName == "" || strings.TrimSpace(definition) == "" {
			return "", Table{}, fmt.Errorf("table %s contains an invalid column definition %q", name, item)
		}
		column, err := parseColumn(adapter, name, columnName, definition, &table)
		if err != nil {
			return "", Table{}, err
		}
		if _, exists := table.Columns[columnName]; exists {
			return "", Table{}, fmt.Errorf("table %s contains duplicate column %s", name, columnName)
		}
		table.Columns[columnName] = column
	}
	for _, constraint := range constraints {
		if err := applyTableConstraint(name, constraint, &table); err != nil {
			return "", Table{}, err
		}
	}
	for columnName, column := range table.Columns {
		if column.PrimaryKey {
			column.Nullable = false
			if adapter == "sqlite" && strings.EqualFold(column.DatabaseType, "integer") {
				column.Generated = true
				column.HasDefault = true
			}
			table.Columns[columnName] = column
		}
	}
	return name, table, nil
}

func parseColumn(adapter, tableName, columnName, definition string, table *Table) (Column, error) {
	typeEnd := constraintStart(definition)
	databaseType := strings.TrimSpace(definition[:typeEnd])
	constraints := definition[typeEnd:]
	if databaseType == "" {
		return Column{}, fmt.Errorf("table %s column %s is missing a type", tableName, columnName)
	}
	portableType, err := portableColumnType(adapter, databaseType)
	if err != nil {
		return Column{}, fmt.Errorf("table %s column %s: %w", tableName, columnName, err)
	}
	upper := strings.ToUpper(constraints)
	column := Column{
		DatabaseType: normalizeDatabaseType(databaseType), Type: portableType,
		Nullable:   !containsWords(upper, "NOT NULL"),
		PrimaryKey: containsWords(upper, "PRIMARY KEY"),
		HasDefault: containsWord(upper, "DEFAULT"),
		Generated:  containsWord(upper, "GENERATED") || containsWord(upper, "IDENTITY") || containsWord(upper, "AUTO_INCREMENT") || strings.Contains(strings.ToLower(databaseType), "serial"),
	}
	if column.Generated {
		column.HasDefault = true
	}
	if column.PrimaryKey {
		column.Nullable = false
		addUnique(table, true, []string{columnName})
	} else if containsWord(upper, "UNIQUE") {
		addUnique(table, false, []string{columnName})
	}
	if referencedTable, referencedColumns, ok := referenceClause(constraints); ok {
		if len(referencedColumns) != 1 {
			return Column{}, fmt.Errorf("table %s column %s has an invalid REFERENCES clause", tableName, columnName)
		}
		foreignKey := ForeignKey{Column: columnName, ReferencedTable: referencedTable, ReferencedColumn: referencedColumns[0]}
		table.ForeignKeys[ForeignKeyKey(foreignKey.Column, foreignKey.ReferencedTable, foreignKey.ReferencedColumn)] = foreignKey
	}
	return column, nil
}

func applyTableConstraint(tableName, definition string, table *Table) error {
	definition = strings.TrimSpace(definition)
	if hasPrefixWord(definition, "CONSTRAINT") {
		_, rest := leadingIdentifier(strings.TrimSpace(definition[len("CONSTRAINT"):]))
		definition = strings.TrimSpace(rest)
	}
	upper := strings.ToUpper(definition)
	switch {
	case strings.HasPrefix(upper, "PRIMARY KEY"):
		columns, err := parenthesizedIdentifiers(definition)
		if err != nil {
			return fmt.Errorf("table %s primary key: %w", tableName, err)
		}
		for _, name := range columns {
			column, exists := table.Columns[name]
			if !exists {
				return fmt.Errorf("table %s primary key references unknown column %s", tableName, name)
			}
			column.PrimaryKey = true
			column.Nullable = false
			table.Columns[name] = column
		}
		addUnique(table, true, columns)
	case strings.HasPrefix(upper, "UNIQUE"):
		columns, err := parenthesizedIdentifiers(definition)
		if err != nil {
			return fmt.Errorf("table %s unique constraint: %w", tableName, err)
		}
		addUnique(table, false, columns)
	case strings.HasPrefix(upper, "FOREIGN KEY"):
		columns, err := parenthesizedIdentifiers(definition)
		if err != nil {
			return fmt.Errorf("table %s foreign key: %w", tableName, err)
		}
		referencedTable, referencedColumns, ok := referenceClause(definition)
		if !ok || len(columns) != len(referencedColumns) {
			return fmt.Errorf("table %s contains an invalid foreign key", tableName)
		}
		for index, column := range columns {
			foreignKey := ForeignKey{Column: column, ReferencedTable: referencedTable, ReferencedColumn: referencedColumns[index]}
			table.ForeignKeys[ForeignKeyKey(foreignKey.Column, foreignKey.ReferencedTable, foreignKey.ReferencedColumn)] = foreignKey
		}
	}
	return nil
}

func parseUniqueIndex(statement string, lock *Lock) error {
	rest := createUniqueIndexPattern.ReplaceAllString(statement, "")
	upper := strings.ToUpper(rest)
	on := strings.Index(upper, " ON ")
	if on < 0 {
		return errorsForSchema("CREATE UNIQUE INDEX is missing ON")
	}
	target := strings.TrimSpace(rest[on+4:])
	open := topLevelIndex(target, '(')
	if open < 0 {
		return errorsForSchema("CREATE UNIQUE INDEX is missing a column list")
	}
	tableName := normalizeIdentifier(strings.TrimSpace(target[:open]))
	table, exists := lock.Tables[tableName]
	if !exists {
		return fmt.Errorf("CREATE UNIQUE INDEX references unknown table %s", tableName)
	}
	close := matchingClose(target, open)
	if close < 0 {
		return fmt.Errorf("CREATE UNIQUE INDEX on %s is missing )", tableName)
	}
	columns := identifiers(target[open+1 : close])
	addUnique(&table, false, columns)
	lock.Tables[tableName] = table
	return nil
}

func addUnique(table *Table, primary bool, columns []string) {
	if len(columns) == 0 {
		return
	}
	if table.UniqueConstraints == nil {
		table.UniqueConstraints = map[string]UniqueConstraint{}
	}
	copy := append([]string(nil), columns...)
	sort.Strings(copy)
	table.UniqueConstraints[ConstraintKey(primary, copy)] = UniqueConstraint{Columns: copy, Primary: primary}
}

func portableColumnType(adapter, databaseType string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(databaseType))
	switch adapter {
	case "sqlite":
		switch {
		case strings.Contains(normalized, "int"):
			return "Integer", nil
		case strings.Contains(normalized, "char"), strings.Contains(normalized, "clob"), strings.Contains(normalized, "text"):
			return "String", nil
		case strings.Contains(normalized, "real"), strings.Contains(normalized, "floa"), strings.Contains(normalized, "doub"):
			return "Float", nil
		case strings.Contains(normalized, "bool"):
			return "Boolean", nil
		case strings.Contains(normalized, "blob"):
			return "Bytes", nil
		}
	case "postgresql":
		base := strings.Fields(strings.NewReplacer("(", " ", ")", " ", ",", " ").Replace(normalized))
		name := ""
		if len(base) > 0 {
			name = base[0]
		}
		switch {
		case strings.Contains(name, "serial"), name == "smallint", name == "integer", name == "bigint", name == "int2", name == "int4", name == "int8":
			return "Integer", nil
		case name == "real", name == "double", name == "float", name == "numeric", name == "decimal":
			return "Float", nil
		case name == "boolean" || name == "bool":
			return "Boolean", nil
		case name == "bytea":
			return "Bytes", nil
		case name == "character", name == "varchar", name == "text", name == "name", name == "uuid", name == "json", name == "jsonb":
			return "String", nil
		}
	case "mysql":
		switch {
		case strings.HasPrefix(normalized, "tinyint(1)"), normalized == "boolean", normalized == "bool":
			return "Boolean", nil
		case strings.Contains(normalized, "int"):
			return "Integer", nil
		case strings.Contains(normalized, "numeric"), strings.Contains(normalized, "decimal"), strings.Contains(normalized, "real"), strings.Contains(normalized, "double"), strings.Contains(normalized, "float"):
			return "Float", nil
		case strings.Contains(normalized, "blob"), strings.Contains(normalized, "binary"):
			return "Bytes", nil
		case strings.Contains(normalized, "char"), strings.Contains(normalized, "text"), strings.Contains(normalized, "json"), strings.Contains(normalized, "enum"), strings.Contains(normalized, "set"):
			return "String", nil
		}
	}
	return "", fmt.Errorf("unsupported %s column type %q", adapter, databaseType)
}

func normalizeDatabaseType(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func tableConstraint(value string) bool {
	upper := strings.ToUpper(strings.TrimSpace(value))
	return strings.HasPrefix(upper, "CONSTRAINT ") || strings.HasPrefix(upper, "PRIMARY KEY") || strings.HasPrefix(upper, "UNIQUE") || strings.HasPrefix(upper, "FOREIGN KEY") || strings.HasPrefix(upper, "CHECK") || strings.HasPrefix(upper, "KEY ") || strings.HasPrefix(upper, "INDEX ")
}

func keywordIndex(value, keyword string) int {
	upper := strings.ToUpper(value)
	quote := byte(0)
	depth := 0
	for index := 0; index < len(value); index++ {
		character := value[index]
		if quote != 0 {
			if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"', '`':
			quote = character
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 && strings.HasPrefix(upper[index:], keyword) {
				before := index == 0 || unicode.IsSpace(rune(value[index-1]))
				after := index+len(keyword) == len(value) || unicode.IsSpace(rune(value[index+len(keyword)]))
				if before && after {
					return index
				}
			}
		}
	}
	return -1
}

var columnConstraintWords = map[string]bool{
	"NOT": true, "NULL": true, "PRIMARY": true, "UNIQUE": true, "DEFAULT": true,
	"GENERATED": true, "IDENTITY": true, "REFERENCES": true, "CHECK": true,
	"COLLATE": true, "CONSTRAINT": true, "AUTO_INCREMENT": true, "COMMENT": true,
}

func constraintStart(definition string) int {
	quote := rune(0)
	depth := 0
	for index := 0; index < len(definition); {
		character := rune(definition[index])
		if quote != 0 {
			if character == quote {
				if index+1 < len(definition) && rune(definition[index+1]) == quote {
					index += 2
					continue
				}
				quote = 0
			}
			index++
			continue
		}
		switch character {
		case '\'', '"', '`':
			quote = character
			index++
		case '(':
			depth++
			index++
		case ')':
			if depth > 0 {
				depth--
			}
			index++
		default:
			if depth == 0 && (unicode.IsLetter(character) || character == '_') {
				start := index
				for index < len(definition) && (unicode.IsLetter(rune(definition[index])) || definition[index] == '_') {
					index++
				}
				if columnConstraintWords[strings.ToUpper(definition[start:index])] {
					return start
				}
				continue
			}
			index++
		}
	}
	return len(definition)
}

func parenthesizedIdentifiers(value string) ([]string, error) {
	open := topLevelIndex(value, '(')
	if open < 0 {
		return nil, errorsForSchema("missing (")
	}
	close := matchingClose(value, open)
	if close < 0 {
		return nil, errorsForSchema("missing )")
	}
	columns := identifiers(value[open+1 : close])
	if len(columns) == 0 {
		return nil, errorsForSchema("column list is empty")
	}
	return columns, nil
}

func referenceClause(value string) (string, []string, bool) {
	upper := strings.ToUpper(value)
	index := strings.Index(upper, "REFERENCES")
	if index < 0 {
		return "", nil, false
	}
	rest := strings.TrimSpace(value[index+len("REFERENCES"):])
	open := topLevelIndex(rest, '(')
	if open < 0 {
		return "", nil, false
	}
	close := matchingClose(rest, open)
	if close < 0 {
		return "", nil, false
	}
	return normalizeIdentifier(strings.TrimSpace(rest[:open])), identifiers(rest[open+1 : close]), true
}

func identifiers(value string) []string {
	parts := splitSQL(value, ',')
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if name := normalizeIdentifier(strings.TrimSpace(part)); name != "" {
			result = append(result, name)
		}
	}
	return result
}

func leadingIdentifier(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	quote := byte(0)
	index := 0
	if value[0] == '"' || value[0] == '`' {
		quote = value[0]
		index++
		for index < len(value) {
			if value[index] == quote {
				if index+1 < len(value) && value[index+1] == quote {
					index += 2
					continue
				}
				index++
				break
			}
			index++
		}
	} else {
		for index < len(value) && !unicode.IsSpace(rune(value[index])) {
			index++
		}
	}
	return normalizeIdentifier(value[:index]), strings.TrimSpace(value[index:])
}

func normalizeIdentifier(value string) string {
	value = strings.TrimSpace(value)
	parts := qualifiedIdentifierParts(value)
	if len(parts) > 0 {
		value = parts[len(parts)-1]
	}
	if len(value) >= 2 && (value[0] == '"' && value[len(value)-1] == '"' || value[0] == '`' && value[len(value)-1] == '`') {
		quote := value[:1]
		value = strings.ReplaceAll(value[1:len(value)-1], quote+quote, quote)
	}
	return value
}

func qualifiedIdentifierParts(value string) []string {
	var result []string
	quote := byte(0)
	start := 0
	for index := 0; index < len(value); index++ {
		character := value[index]
		if quote != 0 {
			if character == quote {
				if index+1 < len(value) && value[index+1] == quote {
					index++
					continue
				}
				quote = 0
			}
			continue
		}
		if character == '"' || character == '`' {
			quote = character
		} else if character == '.' {
			result = append(result, strings.TrimSpace(value[start:index]))
			start = index + 1
		}
	}
	result = append(result, strings.TrimSpace(value[start:]))
	return result
}

func splitSQL(value string, separator rune) []string {
	var result []string
	quote := rune(0)
	depth := 0
	start := 0
	for index, character := range value {
		if quote != 0 {
			if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"', '`':
			quote = character
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if character == separator && depth == 0 {
				result = append(result, value[start:index])
				start = index + 1
			}
		}
	}
	result = append(result, value[start:])
	return result
}

func topLevelIndex(value string, target rune) int {
	quote := rune(0)
	depth := 0
	for index, character := range value {
		if quote != 0 {
			if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"', '`':
			quote = character
		case '(':
			if target == '(' && depth == 0 {
				return index
			}
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if character == target && depth == 0 {
				return index
			}
		}
	}
	return -1
}

func matchingClose(value string, open int) int {
	quote := byte(0)
	depth := 0
	for index := open; index < len(value); index++ {
		character := value[index]
		if quote != 0 {
			if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"', '`':
			quote = character
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func hasPrefixWord(value, word string) bool {
	upper := strings.ToUpper(strings.TrimSpace(value))
	return upper == word || strings.HasPrefix(upper, word+" ")
}

func containsWords(value, words string) bool {
	return strings.Contains(" "+strings.Join(strings.Fields(value), " ")+" ", " "+words+" ")
}

func containsWord(value, word string) bool { return containsWords(value, word) }

func stripSQLComments(value string) string {
	var result strings.Builder
	quote := byte(0)
	for index := 0; index < len(value); {
		if quote != 0 {
			result.WriteByte(value[index])
			if value[index] == quote {
				if index+1 < len(value) && value[index+1] == quote {
					result.WriteByte(value[index+1])
					index += 2
					continue
				}
				quote = 0
			}
			index++
			continue
		}
		if value[index] == '\'' || value[index] == '"' || value[index] == '`' {
			quote = value[index]
			result.WriteByte(value[index])
			index++
			continue
		}
		if index+1 < len(value) && value[index:index+2] == "--" {
			for index < len(value) && value[index] != '\n' {
				index++
			}
			result.WriteByte('\n')
			if index < len(value) {
				index++
			}
			continue
		}
		if index+1 < len(value) && value[index:index+2] == "/*" {
			index += 2
			for index+1 < len(value) && value[index:index+2] != "*/" {
				if value[index] == '\n' {
					result.WriteByte('\n')
				}
				index++
			}
			if index+1 < len(value) {
				index += 2
			}
			continue
		}
		result.WriteByte(value[index])
		index++
	}
	return result.String()
}

func sortedKeys[T any](values map[string]T) []string {
	result := make([]string, 0, len(values))
	for name := range values {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}
