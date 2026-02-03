package Entities

import (
	"database/sql"
	"fmt"
)

type CustomField struct {
	FieldName  string      `json:"field_name"`
	FieldValue interface{} `json:"field_value"`
}

type CustomFieldConfig struct {
	MachineFieldName string
	HumanFieldName   string
	FieldType        string
	FieldValue       interface{}
}

type CustomFieldData struct {
	HumanFieldName string `json:"human_field_name"`
	FieldValue     string `json:"field_value"`
}

type CustomFieldEntity struct {
	CustomFields []CustomField       `json:"custom_fields"`
	FieldConfig  []CustomFieldConfig `json:"field_config"`
}

func getCustomFields(entity CustomFieldEntity) map[string]CustomFieldData {
	// Create a map for faster lookup of custom field values
	valuesMap := make(map[string]interface{})
	for _, field := range entity.CustomFields {
		valuesMap[field.FieldName] = field.FieldValue
	}

	result := make(map[string]CustomFieldData)

	for _, config := range entity.FieldConfig {
		if val, ok := valuesMap[config.MachineFieldName]; ok {
			// Convert interface{} to string
			strValue := fmt.Sprintf("%v", val)
			result[config.MachineFieldName] = CustomFieldData{
				HumanFieldName: config.HumanFieldName,
				FieldValue:     strValue,
			}
		}
	}

	return result
}

func GenerateEntityTables(entity CustomFieldEntity, entityName string, db *sql.DB) error {
	customFieldMainTableSQL := "CREATE TABLE IF NOT EXISTS " + entityName + "_main (" +
		"entity_id INT," +
		"field_machine_name VARCHAR(255)," +
		"field_type INT," +
		"value_int INT," +
		"value_decimal DECIMAL(10,2)," +
		"value_string VARCHAR(255)," +
		"value_text TEXT," +
		"value_date DATETIME)"

	customFieldFlattenedTableSQL := "CREATE TABLE IF NOT EXISTS " + entityName + "_flattened (" +
		"entity_id INT"

	fieldTypeMap := map[string]string{
		"int":     "INT",
		"decimal": "DECIMAL(10,2)",
		"string":  "VARCHAR(255)",
		"text":    "TEXT",
		"date":    "DATETIME",
	}

	valueColumnMap := map[string]string{
		"int":     "value_int",
		"decimal": "value_decimal",
		"string":  "value_string",
		"text":    "value_text",
		"date":    "value_date",
	}

	for _, config := range entity.FieldConfig {
		valCol := valueColumnMap[config.FieldType]
		if valCol == "" {
			valCol = "value_string"
		}
		customFieldFlattenedTableSQL +=
			", " + config.MachineFieldName + " " + fieldTypeMap[config.FieldType] +
				" AS (SELECT " + valCol + " FROM " + entityName + "_main WHERE " + entityName + "_main.entity_id = entity_id AND field_machine_name = '" + config.MachineFieldName + "') PERSISTENT"
	}
	customFieldFlattenedTableSQL += ")"

	if _, err := db.Exec(customFieldMainTableSQL); err != nil {
		return fmt.Errorf("error creating main table: %w", err)
	}
	if _, err := db.Exec(customFieldFlattenedTableSQL); err != nil {
		return fmt.Errorf("error creating flattened table: %w", err)
	}
	return nil
}

func UpdateFlattenedTable(entity CustomFieldEntity, entityName string, db *sql.DB) error {
	tableName := entityName + "_flattened"

	// 1. Get existing columns from the database to avoid trying to add duplicates.
	// This query works for MySQL/MariaDB, which matches the syntax used in GenerateEntityTables.
	rows, err := db.Query("SELECT column_name FROM information_schema.columns WHERE table_name = ? AND table_schema = DATABASE()", tableName)
	if err != nil {
		return fmt.Errorf("failed to query existing columns: %w", err)
	}
	defer rows.Close()

	existingColumns := make(map[string]bool)
	for rows.Next() {
		var colName string
		if err := rows.Scan(&colName); err != nil {
			return fmt.Errorf("failed to scan column name: %w", err)
		}
		existingColumns[colName] = true
	}

	fieldTypeMap := map[string]string{
		"int":     "INT",
		"decimal": "DECIMAL(10,2)",
		"string":  "VARCHAR(255)",
		"text":    "TEXT",
		"date":    "DATETIME",
	}

	valueColumnMap := map[string]string{
		"int":     "value_int",
		"decimal": "value_decimal",
		"string":  "value_string",
		"text":    "value_text",
		"date":    "value_date",
	}

	// Track fields present in the current configuration
	configFieldNames := make(map[string]bool)

	// 2. Iterate over config and add missing columns
	for _, config := range entity.FieldConfig {
		configFieldNames[config.MachineFieldName] = true
		if !existingColumns[config.MachineFieldName] {
			sqlType := fieldTypeMap[config.FieldType]
			if sqlType == "" {
				sqlType = "VARCHAR(255)" // Default fallback
			}

			valCol := valueColumnMap[config.FieldType]
			if valCol == "" {
				valCol = "value_string"
			}

			// Note: Table and column names cannot be parameterized in SQL.
			// Ensure MachineFieldName is sanitized in production to prevent SQL injection.
			alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s AS (SELECT %s FROM %s_main WHERE %s_main.entity_id = entity_id AND field_machine_name = '%s') PERSISTENT",
				tableName, config.MachineFieldName, sqlType, valCol, entityName, entityName, config.MachineFieldName)

			if _, err := db.Exec(alterSQL); err != nil {
				return fmt.Errorf("failed to add column %s: %w", config.MachineFieldName, err)
			}
		}
	}

	// 3. Remove columns that are no longer in the config
	for colName := range existingColumns {
		// Skip the primary identifier column so we don't delete the ID
		if colName == "entity_id" {
			continue
		}

		if !configFieldNames[colName] {
			alterSQL := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", tableName, colName)
			if _, err := db.Exec(alterSQL); err != nil {
				return fmt.Errorf("failed to drop column %s: %w", colName, err)
			}
		}
	}

	return nil
}
