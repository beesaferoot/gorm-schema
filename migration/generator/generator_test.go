package generator

import (
	"os"
	"strings"
	"testing"

	"github.com/beesaferoot/gorm-migrate/migration/diff"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestGenerateCreateTableSQL_ForeignKey(t *testing.T) {
	gen := NewGenerator("migrations")
	table := diff.TableDiff{
		Schema: &schema.Schema{Table: "orders"},
		FieldsToAdd: []*schema.Field{
			{DBName: "id", DataType: "int", PrimaryKey: true, NotNull: true},
			{DBName: "user_id", DataType: "int", NotNull: true},
		},
		ForeignKeysToAdd: []*schema.Relationship{
			{
				Field:  &schema.Field{DBName: "user_id"},
				Schema: &schema.Schema{Table: "users"},
				References: []*schema.Reference{
					{
						PrimaryKey: &schema.Field{
							DBName: "id",
							Schema: &schema.Schema{Table: "users"},
						},
						ForeignKey: &schema.Field{DBName: "user_id"},
					},
				},
			},
		},
	}

	sql := gen.generateCreateTableSQL(table)
	require.Contains(t, sql, "CONSTRAINT fk_orders_user_id_fkey FOREIGN KEY (\"user_id\") REFERENCES \"users\"(id) ON DELETE CASCADE")
	require.Contains(t, sql, "CREATE TABLE \"orders\" (")
	require.NotContains(t, sql, "DEFAULT NULL\n\tDEFAULT NULL")
}

func TestGenerateCreateTableSQL_Indexes(t *testing.T) {
	gen := NewGenerator("migrations")
	table := diff.TableDiff{
		Schema: &schema.Schema{Table: "products"},
		FieldsToAdd: []*schema.Field{
			{DBName: "id", DataType: "int", PrimaryKey: true, NotNull: true},
			{DBName: "sku", DataType: "string", NotNull: true},
			{DBName: "category_id", DataType: "int", NotNull: true},
		},
		IndexesToAdd: []*schema.Index{
			{
				Name:   "products_sku_unique",
				Fields: []schema.IndexOption{{Field: &schema.Field{DBName: "sku"}}},
				Option: "UNIQUE",
			},
			{
				Name:   "products_category_id_idx",
				Fields: []schema.IndexOption{{Field: &schema.Field{DBName: "category_id"}}},
				Option: "",
			},
		},
	}

	sql := gen.generateCreateTableSQL(table)
	require.Contains(t, sql, "CONSTRAINT products_sku_unique UNIQUE (\"sku\")")
	require.Contains(t, sql, "CREATE INDEX products_category_id_idx ON \"products\" (\"category_id\");")
}

func TestGenerateCreateTableSQL_Formatting(t *testing.T) {
	gen := NewGenerator("migrations")
	table := diff.TableDiff{
		Schema: &schema.Schema{Table: "test_table"},
		FieldsToAdd: []*schema.Field{
			{DBName: "id", DataType: "int", PrimaryKey: true, NotNull: true},
			{DBName: "name", DataType: "string", NotNull: true},
		},
	}

	sql := gen.generateCreateTableSQL(table)
	require.Contains(t, sql, "CREATE TABLE \"test_table\" (")
	require.Contains(t, sql, "id SERIAL NOT NULL PRIMARY KEY")
	require.Contains(t, sql, "name varchar(255) NOT NULL")
	require.NotContains(t, sql, ",\n\n")
	require.NotContains(t, sql, ",\n);")
}

func cleanupTestMigrations(t *testing.T, pattern string) {
	files, err := os.ReadDir("migrations")
	if err != nil {
		return
	}
	for _, file := range files {
		if strings.Contains(file.Name(), pattern) {
			_ = os.Remove("migrations/" + file.Name())
		}
	}
}

func TestGenerateMigration_FullProcess(t *testing.T) {
	t.Cleanup(func() { cleanupTestMigrations(t, "test_migration") })
	gen := NewGenerator("migrations")
	schemaDiff := &diff.SchemaDiff{
		TablesToCreate: []diff.TableDiff{
			{
				Schema: &schema.Schema{Table: "users"},
				FieldsToAdd: []*schema.Field{
					{DBName: "id", DataType: "int", PrimaryKey: true, NotNull: false},
					{DBName: "name", DataType: "string", NotNull: false},
				},
			},
			{
				Schema: &schema.Schema{Table: "orders"},
				FieldsToAdd: []*schema.Field{
					{DBName: "id", DataType: "int", PrimaryKey: true, NotNull: false},
					{DBName: "user_id", DataType: "int", NotNull: false},
				},
				ForeignKeysToAdd: []*schema.Relationship{
					{
						Field:  &schema.Field{DBName: "user_id"},
						Schema: &schema.Schema{Table: "users"},
					},
				},
			},
		},
	}

	gen.SetSchemaDiff(schemaDiff)
	err := gen.CreateMigration("test_migration")
	require.NoError(t, err)

	// List the migrations directory to find the generated file
	files, err := os.ReadDir("migrations")
	require.NoError(t, err)
	var migrationFile string
	for _, file := range files {
		if strings.Contains(file.Name(), "test_migration") {
			migrationFile = file.Name()
			break
		}
	}
	require.NotEmpty(t, migrationFile, "Generated migration file not found")

	// Read the generated migration file
	content, err := os.ReadFile("migrations/" + migrationFile)
	require.NoError(t, err)
	sql := string(content)

	// Verify the generated SQL
	require.Contains(t, sql, "CREATE TABLE \"users\" (")
	require.Contains(t, sql, "CREATE TABLE \"orders\" (")
	require.Contains(t, sql, "CONSTRAINT fk_orders_user_id_fkey")
	require.Contains(t, sql, "FOREIGN KEY (\"user_id\")")
	require.Contains(t, sql, "REFERENCES \"users\"(id)")
	require.Contains(t, sql, "ON DELETE CASCADE")
}

func TestGenerateMigration_EmptyDiff(t *testing.T) {
	t.Cleanup(func() { cleanupTestMigrations(t, "empty_migration") })
	gen := NewGenerator("migrations")
	schemaDiff := &diff.SchemaDiff{} // Empty diff
	gen.SetSchemaDiff(schemaDiff)
	err := gen.CreateMigration("empty_migration")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no schema changes detected")
}

func TestGenerateMigration_MissingColumns(t *testing.T) {
	t.Cleanup(func() { cleanupTestMigrations(t, "missing_columns_migration") })
	gen := NewGenerator("migrations")
	schemaDiff := &diff.SchemaDiff{
		TablesToCreate: []diff.TableDiff{
			{
				Schema: &schema.Schema{Table: "test_table"},
				// No columns added
			},
		},
	}
	gen.SetSchemaDiff(schemaDiff)
	err := gen.CreateMigration("missing_columns_migration")
	require.NoError(t, err)
	// Verify the generated migration file handles missing columns gracefully
}

func TestGenerateMigration_InvalidTableName(t *testing.T) {
	t.Cleanup(func() { cleanupTestMigrations(t, "invalid_table_migration") })
	gen := NewGenerator("migrations")
	schemaDiff := &diff.SchemaDiff{
		TablesToCreate: []diff.TableDiff{
			{
				Schema: &schema.Schema{Table: ""}, // Empty table name
				FieldsToAdd: []*schema.Field{
					{DBName: "id", DataType: "int", PrimaryKey: true, NotNull: false},
				},
			},
		},
	}
	gen.SetSchemaDiff(schemaDiff)
	err := gen.CreateMigration("invalid_table_migration")
	require.Error(t, err)
	require.Contains(t, err.Error(), "table name cannot be empty")
}

func TestGenerateMigration_InvalidColumnType(t *testing.T) {
	t.Cleanup(func() { cleanupTestMigrations(t, "invalid_column_migration") })
	gen := NewGenerator("migrations")
	schemaDiff := &diff.SchemaDiff{
		TablesToCreate: []diff.TableDiff{
			{
				Schema: &schema.Schema{Table: "test_table"},
				FieldsToAdd: []*schema.Field{
					{DBName: "id", DataType: "invalid_type", PrimaryKey: true, NotNull: false},
				},
			},
		},
	}
	gen.SetSchemaDiff(schemaDiff)
	err := gen.CreateMigration("invalid_column_migration")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported column type")
}

func TestGenerateMigration_MissingForeignKeyReference(t *testing.T) {
	t.Cleanup(func() { cleanupTestMigrations(t, "missing_fk_migration") })
	gen := NewGenerator("migrations")
	schemaDiff := &diff.SchemaDiff{
		TablesToCreate: []diff.TableDiff{
			{
				Schema: &schema.Schema{Table: "orders"},
				FieldsToAdd: []*schema.Field{
					{DBName: "id", DataType: "int", PrimaryKey: true, NotNull: false},
					{DBName: "user_id", DataType: "int", NotNull: false},
				},
				ForeignKeysToAdd: []*schema.Relationship{
					{
						Name:   "orders_user_id_fkey",
						Field:  &schema.Field{DBName: "user_id"},
						Schema: &schema.Schema{Table: "nonexistent_table"}, // Table doesn't exist
					},
				},
			},
		},
	}
	gen.SetSchemaDiff(schemaDiff)
	err := gen.CreateMigration("missing_fk_migration")
	require.Error(t, err)
	require.Contains(t, err.Error(), "table nonexistent_table not found")
}

func TestGenerateMigration_DuplicateColumnNames(t *testing.T) {
	t.Cleanup(func() { cleanupTestMigrations(t, "duplicate_columns_migration") })
	gen := NewGenerator("migrations")
	schemaDiff := &diff.SchemaDiff{
		TablesToCreate: []diff.TableDiff{
			{
				Schema: &schema.Schema{Table: "test_table"},
				FieldsToAdd: []*schema.Field{
					{DBName: "id", DataType: "int", PrimaryKey: true, NotNull: false},
					{DBName: "id", DataType: "int", PrimaryKey: false, NotNull: false}, // Duplicate column name
				},
			},
		},
	}
	gen.SetSchemaDiff(schemaDiff)
	err := gen.CreateMigration("duplicate_columns_migration")
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate column name")
}

func TestGenerateMigration_InvalidIndexDefinition(t *testing.T) {
	t.Cleanup(func() { cleanupTestMigrations(t, "invalid_index_migration") })
	gen := NewGenerator("migrations")
	schemaDiff := &diff.SchemaDiff{
		TablesToCreate: []diff.TableDiff{
			{
				Schema: &schema.Schema{Table: "test_table"},
				FieldsToAdd: []*schema.Field{
					{DBName: "id", DataType: "int", PrimaryKey: true, NotNull: false},
					{DBName: "name", DataType: "string", NotNull: false},
				},
				IndexesToAdd: []*schema.Index{
					{
						Name:   "invalid_index",
						Fields: []schema.IndexOption{{Field: &schema.Field{DBName: "nonexistent_column"}}}, // Column doesn't exist
						Option: "UNIQUE",
					},
				},
			},
		},
	}
	gen.SetSchemaDiff(schemaDiff)
	err := gen.CreateMigration("invalid_index_migration")
	require.Error(t, err)
	require.Contains(t, err.Error(), "index references non-existent column")
}

func TestGenerateMigration_ComplexRelationships(t *testing.T) {
	t.Cleanup(func() { cleanupTestMigrations(t, "complex_relationships_migration") })
	gen := NewGenerator("migrations")
	schemaDiff := &diff.SchemaDiff{
		TablesToCreate: []diff.TableDiff{
			{
				Schema: &schema.Schema{Table: "users"},
				FieldsToAdd: []*schema.Field{
					{DBName: "id", DataType: "int", PrimaryKey: true, NotNull: false},
					{DBName: "name", DataType: "string", NotNull: false},
				},
			},
			{
				Schema: &schema.Schema{Table: "orders"},
				FieldsToAdd: []*schema.Field{
					{DBName: "id", DataType: "int", PrimaryKey: true, NotNull: false},
					{DBName: "user_id", DataType: "int", NotNull: false},
				},
				ForeignKeysToAdd: []*schema.Relationship{
					{
						Name:   "orders_user_id_fkey",
						Field:  &schema.Field{DBName: "user_id"},
						Schema: &schema.Schema{Table: "users"},
					},
				},
			},
			{
				Schema: &schema.Schema{Table: "order_items"},
				FieldsToAdd: []*schema.Field{
					{DBName: "id", DataType: "int", PrimaryKey: true, NotNull: false},
					{DBName: "order_id", DataType: "int", NotNull: false},
					{DBName: "product_id", DataType: "int", NotNull: false},
				},
				ForeignKeysToAdd: []*schema.Relationship{
					{
						Name:   "order_items_order_id_fkey",
						Field:  &schema.Field{DBName: "order_id"},
						Schema: &schema.Schema{Table: "orders"},
					},
				},
			},
		},
	}
	gen.SetSchemaDiff(schemaDiff)
	err := gen.CreateMigration("complex_relationships_migration")
	require.NoError(t, err)

	// List the migrations directory to find the generated file
	files, err := os.ReadDir("migrations")
	require.NoError(t, err)
	var migrationFile string
	for _, file := range files {
		if strings.Contains(file.Name(), "complex_relationships_migration") {
			migrationFile = file.Name()
			break
		}
	}
	require.NotEmpty(t, migrationFile, "Generated migration file not found")

	// Read the generated migration file
	content, err := os.ReadFile("migrations/" + migrationFile)
	require.NoError(t, err)
	sql := string(content)

	// Verify the generated SQL
	require.Contains(t, sql, "CREATE TABLE \"users\" (")
	require.Contains(t, sql, "CREATE TABLE \"orders\" (")
	require.Contains(t, sql, "CREATE TABLE \"order_items\" (")
	require.Contains(t, sql, "CONSTRAINT fk_orders_user_id_fkey")
	require.Contains(t, sql, "FOREIGN KEY (\"user_id\")")
	require.Contains(t, sql, "REFERENCES \"users\"(id)")
	require.Contains(t, sql, "CONSTRAINT fk_order_items_order_id_fkey")
	require.Contains(t, sql, "FOREIGN KEY (\"order_id\")")
	require.Contains(t, sql, "REFERENCES \"orders\"(id)")
	require.Contains(t, sql, "ON DELETE CASCADE")
}

// createTestDB creates a test database for unit tests
func createTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func createTestSchema(tableName string, fields []*schema.Field) *schema.Schema {
	return &schema.Schema{
		Name:   tableName,
		Table:  tableName,
		Fields: fields,
	}
}

func TestDownMigrationGeneration(t *testing.T) {
	t.Run("Add field generates DROP COLUMN in Down", func(t *testing.T) {
		currentSchema := createTestSchema("users", []*schema.Field{
			{Name: "id", DBName: "id", DataType: "uint", PrimaryKey: true, AutoIncrement: true},
			{Name: "name", DBName: "name", DataType: "string"},
		})
		targetSchema := createTestSchema("users", []*schema.Field{
			{Name: "id", DBName: "id", DataType: "uint", PrimaryKey: true, AutoIncrement: true},
			{Name: "name", DBName: "name", DataType: "string"},
			{Name: "email", DBName: "email", DataType: "string"},
		})
		comparer := diff.NewSchemaComparer(createTestDB(t))
		diffResult := comparer.CompareTable(currentSchema, targetSchema)
		// Debug output for diagnosis
		t.Logf("FieldsToAdd: %+v", diffResult.FieldsToAdd)
		t.Logf("FieldsToDrop: %+v", diffResult.FieldsToDrop)
		t.Logf("FieldsToModify: %+v", diffResult.FieldsToModify)
		g := &Generator{SchemaDiff: &diff.SchemaDiff{TablesToModify: []diff.TableDiff{diffResult}}}
		upSQL := g.generateModifyTableSQL(diffResult)
		fullUpSQL := strings.Join(upSQL, " ")
		downSQL := g.generateDownSQL()
		t.Logf("Up SQL: %s", fullUpSQL)
		t.Logf("Down SQL: %s", downSQL)
		if !strings.Contains(fullUpSQL, "ADD COLUMN \"email\"") {
			t.Errorf("Up migration should add column email")
		}
		if !strings.Contains(downSQL, "DROP COLUMN \"email\"") {
			t.Errorf("Down migration should drop column email")
		}
	})

	t.Run("Remove field generates ADD COLUMN in Down", func(t *testing.T) {
		currentSchema := createTestSchema("users", []*schema.Field{
			{Name: "id", DBName: "id", DataType: "uint", PrimaryKey: true, AutoIncrement: true},
			{Name: "name", DBName: "name", DataType: "string"},
			{Name: "email", DBName: "email", DataType: "string"},
		})
		targetSchema := createTestSchema("users", []*schema.Field{
			{Name: "id", DBName: "id", DataType: "uint", PrimaryKey: true, AutoIncrement: true},
			{Name: "name", DBName: "name", DataType: "string"},
		})
		comparer := diff.NewSchemaComparer(createTestDB(t))
		diffResult := comparer.CompareTable(currentSchema, targetSchema)
		// Debug output for diagnosis
		t.Logf("FieldsToAdd: %+v", diffResult.FieldsToAdd)
		t.Logf("FieldsToDrop: %+v", diffResult.FieldsToDrop)
		t.Logf("FieldsToModify: %+v", diffResult.FieldsToModify)
		g := &Generator{SchemaDiff: &diff.SchemaDiff{TablesToModify: []diff.TableDiff{diffResult}}}
		upSQL := g.generateModifyTableSQL(diffResult)
		fullUpSQL := strings.Join(upSQL, " ")
		downSQL := g.generateDownSQL()
		t.Logf("Up SQL: %s", fullUpSQL)
		t.Logf("Down SQL: %s", downSQL)
		if !strings.Contains(fullUpSQL, "DROP COLUMN \"email\"") {
			t.Errorf("Up migration should drop column email")
		}
		if !strings.Contains(downSQL, "ADD COLUMN \"email\"") {
			t.Errorf("Down migration should add column email")
		}
	})

	t.Run("Modify field generates comment in Down", func(t *testing.T) {
		currentSchema := createTestSchema("users", []*schema.Field{
			{Name: "id", DBName: "id", DataType: "uint", PrimaryKey: true, AutoIncrement: true},
			{Name: "name", DBName: "name", DataType: "string"},
			{Name: "age", DBName: "age", DataType: "int"},
		})
		targetSchema := createTestSchema("users", []*schema.Field{
			{Name: "id", DBName: "id", DataType: "uint", PrimaryKey: true, AutoIncrement: true},
			{Name: "name", DBName: "name", DataType: "string"},
			{Name: "age", DBName: "age", DataType: "string"}, // type changed
		})
		comparer := diff.NewSchemaComparer(createTestDB(t))
		diffResult := comparer.CompareTable(currentSchema, targetSchema)
		// Debug output for diagnosis
		t.Logf("FieldsToAdd: %+v", diffResult.FieldsToAdd)
		t.Logf("FieldsToDrop: %+v", diffResult.FieldsToDrop)
		t.Logf("FieldsToModify: %+v", diffResult.FieldsToModify)
		g := &Generator{SchemaDiff: &diff.SchemaDiff{TablesToModify: []diff.TableDiff{diffResult}}}
		upSQL := g.generateModifyTableSQL(diffResult)
		fullUpSQL := strings.Join(upSQL, " ")
		downSQL := g.generateDownSQL()
		t.Logf("Up SQL: %s", fullUpSQL)
		t.Logf("Down SQL: %s", downSQL)
		if !strings.Contains(fullUpSQL, "ALTER COLUMN \"age\"") {
			t.Errorf("Up migration should alter column age")
		}
		if !strings.Contains(downSQL, "-- TODO: Reverse modification for column age") {
			t.Errorf("Down migration should include a comment for manual intervention")
		}
	})
}
func TestGenerateCreateTableSQL_DuplicateIndexColumns(t *testing.T) {
	gen := NewGenerator("migrations")
	table := diff.TableDiff{
		Schema: &schema.Schema{Table: "products"},
		FieldsToAdd: []*schema.Field{
			{DBName: "id", DataType: "int", PrimaryKey: true, NotNull: true},
			{DBName: "name", DataType: "string", NotNull: true},
		},
		IndexesToAdd: []*schema.Index{
			{
				Name:   "idx_products_name",
				Fields: []schema.IndexOption{{Field: &schema.Field{DBName: "name"}}, {Field: &schema.Field{DBName: "name"}}}, // Duplicate
			},
		},
	}

	sql := gen.generateCreateTableSQL(table)
	// Should only contain "name" once
	require.Contains(t, sql, "CREATE INDEX idx_products_name ON \"products\" (\"name\");")
	require.NotContains(t, sql, "(\"name\", \"name\")")
}

func TestGenerateUpSQL_SchemaCreation(t *testing.T) {
	gen := NewGenerator("migrations")
	table := diff.TableDiff{
		Schema: &schema.Schema{Table: "custom_schema.users"},
		FieldsToAdd: []*schema.Field{
			{DBName: "id", DataType: "int", PrimaryKey: true, NotNull: true},
		},
	}
	gen.SchemaDiff = &diff.SchemaDiff{
		TablesToCreate: []diff.TableDiff{table},
	}

	sql, err := gen.generateUpSQL()
	require.NoError(t, err)

	// Should contain schema creation
	require.Contains(t, sql, "CREATE SCHEMA IF NOT EXISTS \"custom_schema\";")
	require.Contains(t, sql, "CREATE TABLE \"custom_schema\".\"users\"")
}

func TestGenerateCreateTableSQL_ForeignKeyTypeMismatch(t *testing.T) {
	gen := NewGenerator("migrations")
	table := diff.TableDiff{
		Schema: &schema.Schema{Table: "orders"},
		FieldsToAdd: []*schema.Field{
			{DBName: "id", DataType: "int", PrimaryKey: true, NotNull: true},
			{DBName: "user_id", DataType: "int", NotNull: true}, // Go type int -> integer (default)
		},
		ForeignKeysToAdd: []*schema.Relationship{
			{
				Field:  &schema.Field{DBName: "user_id"},
				Schema: &schema.Schema{Table: "users"},
				References: []*schema.Reference{
					{
						PrimaryKey: &schema.Field{
							DBName:   "id",
							DataType: "uint", // Referenced PK is uint (BIGSERIAL -> bigint)
							Schema:   &schema.Schema{Table: "users"},
						},
						ForeignKey: &schema.Field{DBName: "user_id"},
					},
				},
			},
		},
	}

	sql := gen.generateCreateTableSQL(table)

	// Should be bigint because referenced PK is uint
	require.Contains(t, sql, "user_id bigint NOT NULL")
	require.NotContains(t, sql, "user_id integer")
}

func TestCreateMigration_DeterministicMetadata(t *testing.T) {
	t.Cleanup(func() { cleanupTestMigrations(t, "deterministic_metadata") })
	gen := NewGenerator("migrations")
	schemaDiff := &diff.SchemaDiff{
		TablesToCreate: []diff.TableDiff{
			{
				Schema: &schema.Schema{Table: "users"},
				FieldsToAdd: []*schema.Field{
					{DBName: "id", DataType: "int", PrimaryKey: true},
				},
			},
		},
	}
	gen.SetSchemaDiff(schemaDiff)

	err := gen.CreateMigration("deterministic_metadata")
	require.NoError(t, err)

	// Read generated file
	files, _ := os.ReadDir("migrations")
	var content string
	for _, f := range files {
		if strings.Contains(f.Name(), "deterministic_metadata") {
			bytes, _ := os.ReadFile("migrations/" + f.Name())
			content = string(bytes)
			break
		}
	}

	// Verify CreatedAt parses the version string
	require.Contains(t, content, "time.Parse(\"20060102150405\",")
	require.NotContains(t, content, "CreatedAt: time.Now()")
}

func TestSanitizeConstraintName(t *testing.T) {
	gen := NewGenerator("migrations")

	// Test basic sanitization
	name := "fk_schema.table_col_fkey"
	sanitized := gen.sanitizeConstraintName(name)
	require.Equal(t, "fk_schema_table_col_fkey", sanitized)

	// Test no change needed
	name2 := "fk_schema_table_col_fkey"
	sanitized2 := gen.sanitizeConstraintName(name2)
	require.Equal(t, "fk_schema_table_col_fkey", sanitized2)
}
