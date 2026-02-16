package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/beesaferoot/gorm-migrate/migration/diff"
)

// Generator helps create new migration files
type Generator struct {
	MigrationsDir string
	SchemaDiff    *diff.SchemaDiff
}

// NewGenerator creates a new migration generator
func NewGenerator(migrationsDir string) *Generator {
	return &Generator{
		MigrationsDir: migrationsDir,
	}
}

// SetSchemaDiff sets the schema diff for the generator
func (g *Generator) SetSchemaDiff(diff *diff.SchemaDiff) {
	g.SchemaDiff = diff
}

// CreateMigration generates a new migration file
func (g *Generator) CreateMigration(name string) error {
	if g.SchemaDiff == nil {
		return fmt.Errorf("schema diff not set")
	}

	// Guard: do not create a migration if there are no changes
	hasChanges := len(g.SchemaDiff.TablesToCreate) > 0 || len(g.SchemaDiff.TablesToDrop) > 0 || len(g.SchemaDiff.TablesToRename) > 0
	for _, tableMod := range g.SchemaDiff.TablesToModify {
		if !tableMod.IsEmpty() {
			hasChanges = true
			break
		}
	}
	if !hasChanges {
		return fmt.Errorf("no schema changes detected")
	}

	// Validate schema diff
	if err := g.validateSchemaDiff(g.SchemaDiff); err != nil {
		return fmt.Errorf("invalid schema diff: %w", err)
	}

	// Create migrations directory if it doesn't exist
	if err := os.MkdirAll(g.MigrationsDir, 0755); err != nil {
		return fmt.Errorf("failed to create migrations directory: %w", err)
	}

	// Generate version using timestamp
	version := time.Now().Format("20060102150405")
	filename := fmt.Sprintf("%s_%s.go", version, name)
	filepath := filepath.Join(g.MigrationsDir, filename)

	// Generate Up and Down SQL statements
	upSQL, err := g.generateUpSQL()
	if err != nil {
		return err
	}
	downSQL := g.generateDownSQL()

	// Create migration file content
	content := fmt.Sprintf(`package migrations

import (
	"github.com/beesaferoot/gorm-migrate/migration"
	"gorm.io/gorm"
	"time"
)

func init() {
	migration.RegisterMigration(&migration.Migration{
		Version:   "%s",
		Name:      "%s",
		CreatedAt: time.Now(),
		Up: func(db *gorm.DB) error {
			%s
			return nil
		},
		Down: func(db *gorm.DB) error {
			%s
			return nil
		},
	})
}
`, version, name, formatSQLAsExec(upSQL), formatSQLAsExec(downSQL))

	// Write the file
	if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to create migration file: %w", err)
	}

	return nil
}

// formatSQLAsExec wraps each full SQL statement in db.Exec with error handling and proper formatting
func formatSQLAsExec(sql string) string {
	if sql == "" {
		return "// No schema changes"
	}
	statements := splitSQLStatements(sql)
	var stmts []string
	for _, stmt := range statements {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" {
			continue
		}
		// Format the SQL statement with proper indentation
		formattedSQL := formatSQLStatement(trimmed)
		stmts = append(stmts, fmt.Sprintf("if err := db.Exec(`%s`).Error; err != nil {\n\t\t\treturn err\n\t\t}", formattedSQL))
	}
	return strings.Join(stmts, "\n\t\t")
}

// formatSQLStatement formats a SQL statement with proper indentation and line breaks
func formatSQLStatement(sql string) string {
	// First, let's properly format the SQL by adding line breaks at key points
	formatted := formatSQLWithLineBreaks(sql)

	// Split the formatted SQL into lines
	lines := strings.Split(formatted, "\n")

	// Process each line with proper indentation
	var formattedLines []string
	indentLevel := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Adjust indent level based on SQL keywords and parentheses
		indentLevel = adjustIndentLevel(trimmed, indentLevel)

		// Add the line with proper indentation
		indent := strings.Repeat("\t", indentLevel)
		formattedLines = append(formattedLines, indent+trimmed)
	}

	return strings.Join(formattedLines, "\n")
}

// formatSQLWithLineBreaks adds line breaks at appropriate points in SQL
func formatSQLWithLineBreaks(sql string) string {
	// Replace common patterns with line breaks
	sql = strings.ReplaceAll(sql, "CREATE TABLE", "\nCREATE TABLE")
	sql = strings.ReplaceAll(sql, "DROP TABLE", "\nDROP TABLE")
	sql = strings.ReplaceAll(sql, "CREATE INDEX", "\nCREATE INDEX")
	sql = strings.ReplaceAll(sql, "CONSTRAINT", "\n\tCONSTRAINT")
	sql = strings.ReplaceAll(sql, "FOREIGN KEY", "\n\t\tFOREIGN KEY")
	sql = strings.ReplaceAll(sql, "REFERENCES", "\n\t\tREFERENCES")
	sql = strings.ReplaceAll(sql, "ON DELETE", "\n\t\tON DELETE")
	sql = strings.ReplaceAll(sql, "DEFAULT", "\n\tDEFAULT")
	sql = strings.ReplaceAll(sql, "UNIQUE", "\n\tUNIQUE")

	// Add line breaks after commas in column lists
	sql = addLineBreaksAfterCommas(sql)

	// Add line breaks after opening parenthesis in CREATE TABLE
	sql = strings.ReplaceAll(sql, "CREATE TABLE (", "CREATE TABLE (\n\t")

	// Clean up multiple spaces and normalize
	sql = strings.ReplaceAll(sql, "  ", " ")
	sql = strings.ReplaceAll(sql, "\n\n", "\n")

	return strings.TrimSpace(sql)
}

// addLineBreaksAfterCommas adds line breaks after commas in column definitions
func addLineBreaksAfterCommas(sql string) string {
	// Only add line breaks after commas that are followed by column names
	// This is a simple heuristic - we look for comma followed by a word that looks like a column name
	re := regexp.MustCompile(`,\s*([a-zA-Z_][a-zA-Z0-9_]*)`)
	return re.ReplaceAllString(sql, ",\n\t$1")
}

// adjustIndentLevel determines the appropriate indent level for a line
func adjustIndentLevel(line string, currentLevel int) int {
	line = strings.ToUpper(strings.TrimSpace(line))

	// Keywords that should increase indent
	if strings.HasPrefix(line, "CREATE TABLE") {
		return 0
	}
	if strings.HasPrefix(line, "DROP TABLE") {
		return 0
	}
	if strings.HasPrefix(line, "CREATE INDEX") {
		return 0
	}
	if strings.HasPrefix(line, "CONSTRAINT") {
		return 1
	}
	if strings.HasPrefix(line, "FOREIGN KEY") {
		return 2
	}
	if strings.HasPrefix(line, "REFERENCES") {
		return 2
	}
	if strings.HasPrefix(line, "ON DELETE") {
		return 2
	}
	if strings.HasPrefix(line, "PRIMARY KEY") {
		return 1
	}
	if strings.HasPrefix(line, "NOT NULL") {
		return 1
	}
	if strings.HasPrefix(line, "DEFAULT") {
		return 1
	}
	if strings.HasPrefix(line, "UNIQUE") {
		return 1
	}

	// Check for closing parenthesis to decrease indent
	if strings.HasPrefix(line, ")") {
		if currentLevel > 0 {
			return currentLevel - 1
		}
		return 0
	}

	// Check if this is a column definition (has a data type)
	if isColumnDefinition(line) {
		return 1
	}

	// Default to current level for other content
	return currentLevel
}

// isColumnDefinition checks if a line looks like a column definition
func isColumnDefinition(line string) bool {
	// Look for common SQL data types
	dataTypes := []string{
		"VARCHAR", "CHAR", "TEXT", "STRING",
		"INT", "INTEGER", "BIGINT", "SMALLINT",
		"FLOAT", "DOUBLE", "DECIMAL", "NUMERIC",
		"BOOLEAN", "BOOL",
		"TIMESTAMP", "TIME", "DATE", "DATETIME",
		"JSON", "JSONB", "BLOB", "BYTEA",
	}

	for _, dataType := range dataTypes {
		if strings.Contains(line, dataType) {
			return true
		}
	}

	return false
}

// splitSQLStatements splits a string into SQL statements by semicolon, preserving multi-line statements
func splitSQLStatements(sql string) []string {
	var stmts []string
	var current strings.Builder
	for _, line := range strings.Split(sql, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		current.WriteString(line)
		if strings.HasSuffix(trimmed, ";") {
			stmts = append(stmts, current.String())
			current.Reset()
		} else {
			current.WriteString("\n")
		}
	}
	if current.Len() > 0 {
		stmts = append(stmts, current.String())
	}
	return stmts
}

// mapGoTypeToSQLType maps Go types to SQL types
func mapGoTypeToSQLType(goType string) string {
	switch goType {
	case "time":
		return "timestamp"
	case "string":
		return "varchar(255)"
	case "int":
		return "integer"
	case "uint":
		return "bigint"
	case "float":
		return "double precision"
	case "bool":
		return "boolean"
	case "json":
		return "jsonb"
	default:
		return goType
	}
}

// mapGoTypeToSQLTypeWithAutoIncrement maps Go types to SQL types, handling auto-increment for primary keys
func mapGoTypeToSQLTypeWithAutoIncrement(goType string, isPrimaryKey bool) string {
	baseType := mapGoTypeToSQLType(goType)

	// For PostgreSQL, use SERIAL types for auto-increment primary keys
	if isPrimaryKey {
		switch goType {
		case "uint":
			return "BIGSERIAL"
		case "int":
			return "SERIAL"
		}
	}

	return baseType
}

// getDefaultValue returns the appropriate default value for a column
func getDefaultValue(colType string, isPrimaryKey bool, tableName string) string {
	// Do not generate sequence for primary key
	if isPrimaryKey {
		return ""
	}

	switch colType {
	case "timestamp":
		return "DEFAULT CURRENT_TIMESTAMP"
	case "boolean":
		return "DEFAULT false"
	case "integer", "bigint", "double precision":
		return "DEFAULT 0"
	case "varchar(255)":
		return "DEFAULT ''"
	case "jsonb":
		return "DEFAULT '{}'::jsonb"
	default:
		return "DEFAULT NULL"
	}
}

// Topological sort for tables based on foreign key dependencies
func topoSortTables(tables []diff.TableDiff) ([]diff.TableDiff, error) {
	tableMap := make(map[string]diff.TableDiff)
	for _, t := range tables {
		tableMap[t.Schema.Table] = t
	}
	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	var sorted []diff.TableDiff
	var visit func(string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("circular dependency detected at table %s", name)
		}
		visiting[name] = true
		t, ok := tableMap[name]
		if !ok {
			return fmt.Errorf("table %s not found", name)
		}
		for _, fk := range t.ForeignKeysToAdd {
			if len(fk.References) < 1 {
				continue
			}
			r := fk.References[0]
			if r != nil && r.PrimaryKey != nil && r.PrimaryKey.Schema != nil {
				referencedTable := r.PrimaryKey.Schema.Table
				if referencedTable != "" && referencedTable != t.Schema.Table {
					if err := visit(referencedTable); err != nil {
						return err
					}
				}
			}
		}
		visiting[name] = false
		visited[name] = true
		sorted = append(sorted, t)
		return nil
	}
	for _, t := range tables {
		if err := visit(t.Schema.Table); err != nil {
			return nil, err
		}
	}
	return sorted, nil
}

// generateUpSQL generates the SQL statements for the Up migration
func (g *Generator) generateUpSQL() (string, error) {
	if g.SchemaDiff == nil {
		return "", nil
	}

	var statements []string

	// Topologically sort tables to create
	tablesToCreate, err := topoSortTables(g.SchemaDiff.TablesToCreate)
	if err != nil {
		return "", err
	}

	// Create tables
	for _, table := range tablesToCreate {
		statements = append(statements, g.generateCreateTableSQL(table))
	}

	// Modify tables
	for _, table := range g.SchemaDiff.TablesToModify {
		statements = append(statements, g.generateModifyTableSQL(table)...)
	}

	return strings.Join(statements, "\n"), nil
}

// generateDownSQL generates the SQL statements for the Down migration
func (g *Generator) generateDownSQL() string {
	if g.SchemaDiff == nil {
		return ""
	}

	var statements []string

	// Drop indexes first
	for _, table := range g.SchemaDiff.TablesToModify {
		for _, idx := range table.IndexesToAdd {
			idxName := idx.Name
			if strings.HasPrefix(idxName, "idx_idx_") {
				idxName = strings.Replace(idxName, "idx_idx_", "idx_", 1)
			}
			statements = append(statements, fmt.Sprintf("DROP INDEX IF EXISTS %s;", idxName))
		}
	}

	// Drop foreign keys
	for _, table := range g.SchemaDiff.TablesToModify {
		for _, fk := range table.ForeignKeysToAdd {
			if fk.Field != nil {
				statements = append(statements, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS fk_%s_%s_fkey;",
					quoteIdentifier(table.Schema.Table),
					table.Schema.Table,
					fk.Field.DBName))
			}
		}
	}

	// Reverse column changes for modified tables
	for _, table := range g.SchemaDiff.TablesToModify {
		tableName := quoteIdentifier(table.Schema.Table)
		// Reverse added columns: drop them
		for _, col := range table.FieldsToAdd {
			statements = append(statements, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", tableName, quoteIdentifier(col.DBName)))
		}
		// Reverse dropped columns: add them back (best guess type)
		for _, col := range table.FieldsToDrop {
			// Try to guess the SQL type, fallback to comment if unknown
			sqlType := mapGoTypeToSQLType(string(col.DataType))
			if sqlType == "" {
				statements = append(statements, fmt.Sprintf("-- TODO: Could not determine type for column %s, please edit manually", col.DBName))
				continue
			}
			colDef := fmt.Sprintf("%s %s", quoteIdentifier(col.DBName), sqlType)
			if col.NotNull {
				colDef += " NOT NULL"
			}
			if col.DefaultValue != "" {
				colDef += fmt.Sprintf(" DEFAULT %v", col.DefaultValue)
			}
			statements = append(statements, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", tableName, colDef))
		}
		// Reverse modified columns: add a comment (manual intervention needed)
		for _, col := range table.FieldsToModify {
			statements = append(statements, fmt.Sprintf("-- TODO: Reverse modification for column %s in table %s manually", col.DBName, table.Schema.Table))
		}
	}

	// Drop tables created in Up
	tablesToDrop, err := topoSortTables(g.SchemaDiff.TablesToCreate)
	if err != nil {
		for i := len(g.SchemaDiff.TablesToCreate) - 1; i >= 0; i-- {
			table := g.SchemaDiff.TablesToCreate[i]
			statements = append(statements, fmt.Sprintf("DROP TABLE IF EXISTS %s;", quoteIdentifier(table.Schema.Table)))
		}
	} else {
		for i := len(tablesToDrop) - 1; i >= 0; i-- {
			table := tablesToDrop[i]
			statements = append(statements, fmt.Sprintf("DROP TABLE IF EXISTS %s;", quoteIdentifier(table.Schema.Table)))
		}
	}

	return strings.Join(statements, "\n")
}

// generateCreateTableSQL generates the SQL for creating a table with proper formatting
func (g *Generator) generateCreateTableSQL(table diff.TableDiff) string {
	var columns []string
	var tableConstraints []string
	var indexSQLs []string

	// Add columns with proper formatting
	for _, col := range table.FieldsToAdd {
		sqlType := mapGoTypeToSQLTypeWithAutoIncrement(string(col.DataType), col.PrimaryKey)
		columnDef := fmt.Sprintf("%s %s", col.DBName, sqlType)
		if col.NotNull {
			columnDef += " NOT NULL"
		}
		if col.PrimaryKey {
			columnDef += " PRIMARY KEY"
		}
		// Add default value if not primary key and not already set
		if !col.PrimaryKey && col.DefaultValue != "" {
			columnDef += fmt.Sprintf(" DEFAULT '%v'", col.DefaultValue)
		}
		columns = append(columns, "    "+columnDef)
	}

	// Add foreign keys as table constraints
	for _, fk := range table.ForeignKeysToAdd {
		if len(fk.References) < 1 {
			continue
		}
		r := fk.References[0]
		if fk.Field != nil && fk.Schema != nil && r != nil {
			fkDef := fmt.Sprintf("CONSTRAINT fk_%s_%s_fkey FOREIGN KEY (%s) REFERENCES %s(id) ON DELETE CASCADE",
				table.Schema.Table,
				r.ForeignKey.DBName,
				quoteIdentifier(fk.References[0].ForeignKey.DBName),
				quoteIdentifier(r.PrimaryKey.Schema.Table))
			tableConstraints = append(tableConstraints, "    "+fkDef)
		}
	}

	// Add unique indexes as table constraints, non-unique as separate statements
	for _, idx := range table.IndexesToAdd {
		idxName := idx.Name
		if strings.HasPrefix(idxName, "idx_idx_") {
			idxName = strings.Replace(idxName, "idx_idx_", "idx_", 1)
		}
		fieldNames := make([]string, len(idx.Fields))
		for i, f := range idx.Fields {
			fieldNames[i] = quoteIdentifier(f.DBName)
		}
		if strings.ToUpper(idx.Option) == "UNIQUE" {
			idxDef := fmt.Sprintf("CONSTRAINT %s UNIQUE (%s)",
				idxName,
				strings.Join(fieldNames, ", "))
			tableConstraints = append(tableConstraints, "    "+idxDef)
		} else {
			indexSQLs = append(indexSQLs, fmt.Sprintf("CREATE INDEX %s ON %s (%s);", idxName, quoteIdentifier(table.Schema.Table), strings.Join(fieldNames, ", ")))
		}
	}

	// Combine columns and constraints, filter out empty lines
	allLines := append(columns, tableConstraints...)
	var nonEmptyLines []string
	for _, line := range allLines {
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "DEFAULT NULL") {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	// Create table SQL
	createTableSQL := fmt.Sprintf("CREATE TABLE %s (\n%s\n);", quoteIdentifier(table.Schema.Table), strings.Join(nonEmptyLines, ",\n"))

	// Combine table and index creation
	var stmts []string
	stmts = append(stmts, createTableSQL)
	stmts = append(stmts, indexSQLs...)

	return strings.Join(stmts, "\n")
}

// generateModifyTableSQL generates the SQL for modifying a table with proper formatting
func (g *Generator) generateModifyTableSQL(table diff.TableDiff) []string {
	var statements []string

	// Add columns with proper formatting
	for _, col := range table.FieldsToAdd {
		sqlType := mapGoTypeToSQLTypeWithAutoIncrement(string(col.DataType), col.PrimaryKey)
		columnDef := fmt.Sprintf("%s %s", quoteIdentifier(col.DBName), sqlType)
		if col.NotNull {
			columnDef += " NOT NULL"
		}
		if col.DefaultValue != "" {
			columnDef += fmt.Sprintf(" DEFAULT %v", col.DefaultValue)
		}
		statements = append(statements, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", quoteIdentifier(table.Schema.Table), columnDef))
	}

	// Drop columns with proper formatting
	for _, col := range table.FieldsToDrop {
		statements = append(statements, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", quoteIdentifier(table.Schema.Table), quoteIdentifier(col.DBName)))
	}

	// Modify columns with proper formatting
	for _, col := range table.FieldsToModify {
		sqlType := mapGoTypeToSQLTypeWithAutoIncrement(string(col.DataType), col.PrimaryKey)
		columnDef := fmt.Sprintf("%s %s", quoteIdentifier(col.DBName), sqlType)
		if col.NotNull {
			columnDef += " NOT NULL"
		}
		if col.DefaultValue != "" {
			columnDef += fmt.Sprintf(" DEFAULT %v", col.DefaultValue)
		}
		statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s;", quoteIdentifier(table.Schema.Table), columnDef))
	}

	// Add foreign keys with proper formatting
	for _, fk := range table.ForeignKeysToAdd {
		if fk.Field != nil && fk.Schema != nil && len(fk.References) > 0 {
			statements = append(statements, fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT fk_%s_%s_fkey FOREIGN KEY (%s) REFERENCES %s(id) ON DELETE CASCADE;",
				quoteIdentifier(table.Schema.Table),
				table.Schema.Table,
				fk.References[0].ForeignKey.DBName,
				quoteIdentifier(fk.References[0].ForeignKey.DBName),
				quoteIdentifier(fk.References[0].PrimaryKey.Schema.Table)),
			)
		}
	}

	// Add indexes with proper formatting
	for _, idx := range table.IndexesToAdd {
		idxName := idx.Name
		if strings.HasPrefix(idxName, "idx_idx_") {
			idxName = strings.Replace(idxName, "idx_idx_", "idx_", 1)
		}
		fieldNames := make([]string, len(idx.Fields))
		for i, f := range idx.Fields {
			fieldNames[i] = quoteIdentifier(f.DBName)
		}
		if strings.ToUpper(idx.Option) == "UNIQUE" {
			statements = append(statements, fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (%s);",
				idxName,
				quoteIdentifier(table.Schema.Table),
				strings.Join(fieldNames, ", ")))
		} else {
			statements = append(statements, fmt.Sprintf("CREATE INDEX %s ON %s (%s);",
				idxName,
				quoteIdentifier(table.Schema.Table),
				strings.Join(fieldNames, ", ")))
		}
	}

	return statements
}

func (g *Generator) validateSchemaDiff(diff *diff.SchemaDiff) error {
	if diff == nil {
		return fmt.Errorf("schema diff cannot be nil")
	}

	// Collect all table names first
	tableNames := make(map[string]bool)
	for _, table := range diff.TablesToCreate {
		tableNames[table.Schema.Table] = true
	}

	columnNames := make(map[string]map[string]bool)

	// Validate tables to create
	for _, table := range diff.TablesToCreate {
		// Validate table name
		if table.Schema.Table == "" {
			return fmt.Errorf("table name cannot be empty")
		}

		// Track columns for this table
		columnNames[table.Schema.Table] = make(map[string]bool)

		// Validate columns
		for _, col := range table.FieldsToAdd {
			// Validate column name
			if col.DBName == "" {
				return fmt.Errorf("column name cannot be empty in table %s", table.Schema.Table)
			}

			// Check for duplicate column names
			if columnNames[table.Schema.Table][col.DBName] {
				return fmt.Errorf("duplicate column name %s in table %s", col.DBName, table.Schema.Table)
			}
			columnNames[table.Schema.Table][col.DBName] = true

			// Validate column type
			if !isValidColumnType(string(col.DataType)) {
				return fmt.Errorf("unsupported column type %s for column %s in table %s", col.DataType, col.DBName, table.Schema.Table)
			}
		}

		// Validate foreign keys
		for _, fk := range table.ForeignKeysToAdd {
			if fk.Field != nil {
				if !columnNames[table.Schema.Table][fk.References[0].ForeignKey.DBName] {
					return fmt.Errorf("foreign key column %s does not exist in table %s", fk.References[0].ForeignKey.DBName, table.Schema.Table)
				}
			}
		}

		// Validate indexes
		for _, idx := range table.IndexesToAdd {
			// Check if indexed columns exist
			for _, col := range idx.Fields {
				if !columnNames[table.Schema.Table][col.DBName] {
					return fmt.Errorf("index references non-existent column %s in table %s", col.DBName, table.Schema.Table)
				}
			}
		}
	}

	return nil
}

func isValidColumnType(columnType string) bool {
	validTypes := map[string]bool{
		"int":       true,
		"integer":   true,
		"bigint":    true,
		"uint":      true,
		"string":    true,
		"text":      true,
		"varchar":   true,
		"bool":      true,
		"boolean":   true,
		"float":     true,
		"float64":   true,
		"decimal":   true,
		"time":      true,
		"timestamp": true,
		"json":      true,
		"jsonb":     true,
		"uuid":      true,
	}

	// Allow parameterized types like decimal(10,2), varchar(255), etc.
	re := regexp.MustCompile(`^([a-zA-Z_]+)(\(.*\))?$`)
	matches := re.FindStringSubmatch(columnType)
	if len(matches) > 1 {
		baseType := strings.ToLower(matches[1])
		if validTypes[baseType] {
			return true
		}
	}
	return false
}

// quoteIdentifier wraps a SQL identifier (table or column name) in double quotes
func quoteIdentifier(name string) string {
	parts := strings.Split(name, ".")
	for i, part := range parts {
		parts[i] = `"` + part + `"`
	}
	return strings.Join(parts, ".")
}
