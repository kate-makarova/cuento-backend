package Services

import (
	"cuento-backend/src/Entities"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

func GetEntity(id int64, className string, db *sql.DB) (interface{}, error) {
	// Basic validation
	for _, r := range className {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return nil, fmt.Errorf("invalid class name")
		}
	}

	// 1. Fetch data as map
	query := fmt.Sprintf("SELECT * FROM %s_base LEFT JOIN %s_flattened ON %s_base.id = %s_flattened.entity_id WHERE %s_base.id = ?", className, className, className, className, className)

	rows, err := db.Query(query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, sql.ErrNoRows
	}

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	vals := make([]interface{}, len(cols))
	for i := range vals {
		vals[i] = new(sql.RawBytes)
	}

	if err := rows.Scan(vals...); err != nil {
		return nil, err
	}

	data := make(map[string]interface{})
	for i, colName := range cols {
		val := vals[i].(*sql.RawBytes)
		if *val == nil {
			continue
		}
		var v interface{}
		if err := json.Unmarshal(*val, &v); err == nil {
			data[colName] = v
		} else {
			data[colName] = string(*val)
		}
	}

	// 2. Instantiate struct
	var entity interface{}
	switch className {
	case "character":
		entity = &Entities.Character{}
	default:
		return nil, fmt.Errorf("unknown entity class: %s", className)
	}

	// 3. Fill struct
	if err := fillEntity(entity, data); err != nil {
		return nil, err
	}

	return entity, nil
}

func fillEntity(entity interface{}, data map[string]interface{}) error {
	v := reflect.ValueOf(entity).Elem()
	t := v.Type()

	usedKeys := make(map[string]bool)

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		fieldName := fieldType.Name

		// Simple mapping: struct field "Name" -> db column "name"
		dbKey := strings.ToLower(fieldName)

		if val, ok := data[dbKey]; ok {
			usedKeys[dbKey] = true
			if err := setField(field, val); err != nil {
				return fmt.Errorf("failed to set field %s: %w", fieldName, err)
			}
		}
	}

	// Handle CustomFields
	// Look for a field of type CustomFieldEntity
	cfField := v.FieldByName("CustomFields")
	if cfField.IsValid() && cfField.Type() == reflect.TypeOf(Entities.CustomFieldEntity{}) {
		var cfSlice []Entities.CustomField
		for key, val := range data {
			if !usedKeys[key] && key != "entity_id" { // Ignore entity_id as it's duplicate of id
				cfSlice = append(cfSlice, Entities.CustomField{
					FieldName:  key,
					FieldValue: val,
				})
			}
		}

		// Set the CustomFields slice in the CustomFieldEntity struct
		cfListField := cfField.FieldByName("CustomFields")
		if cfListField.IsValid() && cfListField.CanSet() {
			cfListField.Set(reflect.ValueOf(cfSlice))
		}
	}

	return nil
}

func setField(field reflect.Value, val interface{}) error {
	if !field.CanSet() {
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		if s, ok := val.(string); ok {
			field.SetString(s)
		} else {
			field.SetString(fmt.Sprintf("%v", val))
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var i int64
		switch v := val.(type) {
		case float64:
			i = int64(v)
		case string:
			n, err := strconv.ParseInt(v, 10, 64)
			if err == nil {
				i = n
			}
		case int:
			i = int64(v)
		}
		field.SetInt(i)
	case reflect.Ptr:
		if val == nil {
			field.Set(reflect.Zero(field.Type()))
			return nil
		}
		elem := reflect.New(field.Type().Elem())
		if err := setField(elem.Elem(), val); err != nil {
			return err
		}
		field.Set(elem)
	default:
		if reflect.TypeOf(val).AssignableTo(field.Type()) {
			field.Set(reflect.ValueOf(val))
		}
	}
	return nil
}

func CreateEntity(className string, entity interface{}, db *sql.DB) (interface{}, error) {
	// Basic validation
	for _, r := range className {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return nil, fmt.Errorf("invalid class name")
		}
	}

	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()

	// 1. Insert into base table
	var cols []string
	var vals []interface{}
	var placeholders []string

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		fieldName := fieldType.Name

		if fieldName == "Id" || fieldName == "CustomFields" {
			continue
		}

		cols = append(cols, strings.ToLower(fieldName))
		vals = append(vals, field.Interface())
		placeholders = append(placeholders, "?")
	}

	var id int64
	if len(cols) > 0 {
		query := fmt.Sprintf("INSERT INTO %s_base (%s) VALUES (%s)", className, strings.Join(cols, ", "), strings.Join(placeholders, ", "))
		res, err := db.Exec(query, vals...)
		if err != nil {
			return nil, fmt.Errorf("failed to insert base entity: %w", err)
		}
		id, err = res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("failed to get insert id: %w", err)
		}

		// Set ID back to struct
		idField := v.FieldByName("Id")
		if idField.IsValid() && idField.CanSet() {
			idField.SetInt(id)
		}
	} else {
		return nil, fmt.Errorf("no base fields to insert")
	}

	// 2. Insert custom fields
	cfField := v.FieldByName("CustomFields")
	if cfField.IsValid() {
		cfListField := cfField.FieldByName("CustomFields")
		if cfListField.IsValid() && cfListField.Kind() == reflect.Slice && cfListField.Len() > 0 {
			// Get column types from flattened table
			rows, err := db.Query(fmt.Sprintf("SELECT * FROM %s_flattened WHERE 1=0", className))
			if err != nil {
				return nil, fmt.Errorf("failed to query custom fields metadata: %w", err)
			}
			defer rows.Close()

			colTypes, err := rows.ColumnTypes()
			if err != nil {
				return nil, fmt.Errorf("failed to get column types: %w", err)
			}

			colTypeMap := make(map[string]string)
			for _, ct := range colTypes {
				colTypeMap[ct.Name()] = ct.DatabaseTypeName()
			}

			stmt, err := db.Prepare(fmt.Sprintf("INSERT INTO %s_main (entity_id, field_machine_name, field_type, value_int, value_decimal, value_string, value_text, value_date) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", className))
			if err != nil {
				return nil, fmt.Errorf("failed to prepare custom field insert: %w", err)
			}
			defer stmt.Close()

			for i := 0; i < cfListField.Len(); i++ {
				cf := cfListField.Index(i)
				fieldName := cf.FieldByName("FieldName").String()
				fieldValue := cf.FieldByName("FieldValue").Interface()

				dbType, ok := colTypeMap[fieldName]
				if !ok {
					continue
				}

				var fieldType string
				var valInt *int
				var valDecimal *float64
				var valString *string
				var valText *string
				var valDate *string

				switch dbType {
				case "INT", "INTEGER", "BIGINT", "SMALLINT", "TINYINT":
					fieldType = "int"
					if v, ok := fieldValue.(float64); ok {
						i := int(v)
						valInt = &i
					} else if v, ok := fieldValue.(int); ok {
						valInt = &v
					}
				case "DECIMAL", "FLOAT", "DOUBLE":
					fieldType = "decimal"
					if v, ok := fieldValue.(float64); ok {
						valDecimal = &v
					}
				case "VARCHAR", "CHAR":
					fieldType = "string"
					if v, ok := fieldValue.(string); ok {
						valString = &v
					}
				case "TEXT", "BLOB":
					fieldType = "text"
					if v, ok := fieldValue.(string); ok {
						valText = &v
					}
				case "DATETIME", "DATE", "TIMESTAMP":
					fieldType = "date"
					if v, ok := fieldValue.(string); ok {
						valDate = &v
					}
				default:
					fieldType = "string"
					if v, ok := fieldValue.(string); ok {
						valString = &v
					}
				}

				_, err := stmt.Exec(id, fieldName, fieldType, valInt, valDecimal, valString, valText, valDate)
				if err != nil {
					return nil, fmt.Errorf("failed to insert custom field %s: %w", fieldName, err)
				}
			}
		}
	}

	return entity, nil
}

func PatchEntity(id int64, className string, updates map[string]interface{}, db *sql.DB) (interface{}, error) {
	// Basic validation
	for _, r := range className {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return nil, fmt.Errorf("invalid class name")
		}
	}

	// 1. Identify base fields
	var entity interface{}
	switch className {
	case "character":
		entity = &Entities.Character{}
	default:
		return nil, fmt.Errorf("unknown entity class: %s", className)
	}

	v := reflect.ValueOf(entity).Elem()
	t := v.Type()

	baseFieldNames := make(map[string]bool)
	for i := 0; i < v.NumField(); i++ {
		fieldName := t.Field(i).Name
		if fieldName != "Id" && fieldName != "CustomFields" {
			baseFieldNames[strings.ToLower(fieldName)] = true
		}
	}

	// 2. Prepare base update
	var baseUpdates []string
	var baseArgs []interface{}

	for key, val := range updates {
		lowerKey := strings.ToLower(key)
		if baseFieldNames[lowerKey] {
			baseUpdates = append(baseUpdates, fmt.Sprintf("%s = ?", lowerKey))
			baseArgs = append(baseArgs, val)
		}
	}

	if len(baseUpdates) > 0 {
		query := fmt.Sprintf("UPDATE %s_base SET %s WHERE id = ?", className, strings.Join(baseUpdates, ", "))
		baseArgs = append(baseArgs, id)
		if _, err := db.Exec(query, baseArgs...); err != nil {
			return nil, fmt.Errorf("failed to update base entity: %w", err)
		}
	}

	// 3. Update custom fields
	if cfVal, ok := updates["custom_fields"]; ok {
		var fields []interface{}
		// Handle nested structure: custom_fields: { custom_fields: [...] }
		if cfMap, ok := cfVal.(map[string]interface{}); ok {
			if fVal, ok := cfMap["custom_fields"]; ok {
				if fList, ok := fVal.([]interface{}); ok {
					fields = fList
				}
			}
		} else if fList, ok := cfVal.([]interface{}); ok {
			fields = fList
		}

		if len(fields) > 0 {
			// Get column types from flattened table
			rows, err := db.Query(fmt.Sprintf("SELECT * FROM %s_flattened WHERE 1=0", className))
			if err != nil {
				return nil, fmt.Errorf("failed to query custom fields metadata: %w", err)
			}
			defer rows.Close()

			colTypes, err := rows.ColumnTypes()
			if err != nil {
				return nil, fmt.Errorf("failed to get column types: %w", err)
			}

			colTypeMap := make(map[string]string)
			for _, ct := range colTypes {
				colTypeMap[ct.Name()] = ct.DatabaseTypeName()
			}

			checkStmt, err := db.Prepare(fmt.Sprintf("SELECT 1 FROM %s_main WHERE entity_id = ? AND field_machine_name = ?", className))
			if err != nil {
				return nil, fmt.Errorf("failed to prepare check statement: %w", err)
			}
			defer checkStmt.Close()

			insertStmt, err := db.Prepare(fmt.Sprintf("INSERT INTO %s_main (entity_id, field_machine_name, field_type, value_int, value_decimal, value_string, value_text, value_date) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", className))
			if err != nil {
				return nil, fmt.Errorf("failed to prepare insert statement: %w", err)
			}
			defer insertStmt.Close()

			updateStmt, err := db.Prepare(fmt.Sprintf("UPDATE %s_main SET field_type = ?, value_int = ?, value_decimal = ?, value_string = ?, value_text = ?, value_date = ? WHERE entity_id = ? AND field_machine_name = ?", className))
			if err != nil {
				return nil, fmt.Errorf("failed to prepare update statement: %w", err)
			}
			defer updateStmt.Close()

			for _, f := range fields {
				fieldMap, ok := f.(map[string]interface{})
				if !ok {
					continue
				}
				fieldName, _ := fieldMap["field_name"].(string)
				fieldValue := fieldMap["field_value"]

				if fieldName == "" {
					continue
				}

				dbType, ok := colTypeMap[fieldName]
				if !ok {
					continue
				}

				var fieldType string
				var valInt *int
				var valDecimal *float64
				var valString *string
				var valText *string
				var valDate *string

				switch dbType {
				case "INT", "INTEGER", "BIGINT", "SMALLINT", "TINYINT":
					fieldType = "int"
					if v, ok := fieldValue.(float64); ok {
						i := int(v)
						valInt = &i
					}
				case "DECIMAL", "FLOAT", "DOUBLE":
					fieldType = "decimal"
					if v, ok := fieldValue.(float64); ok {
						valDecimal = &v
					}
				case "VARCHAR", "CHAR":
					fieldType = "string"
					if v, ok := fieldValue.(string); ok {
						valString = &v
					}
				case "TEXT", "BLOB":
					fieldType = "text"
					if v, ok := fieldValue.(string); ok {
						valText = &v
					}
				case "DATETIME", "DATE", "TIMESTAMP":
					fieldType = "date"
					if v, ok := fieldValue.(string); ok {
						valDate = &v
					}
				default:
					fieldType = "string"
					if v, ok := fieldValue.(string); ok {
						valString = &v
					}
				}

				var exists int
				err := checkStmt.QueryRow(id, fieldName).Scan(&exists)
				if err != nil && err != sql.ErrNoRows {
					return nil, fmt.Errorf("failed to check custom field existence: %w", err)
				}

				if err == sql.ErrNoRows {
					_, err = insertStmt.Exec(id, fieldName, fieldType, valInt, valDecimal, valString, valText, valDate)
				} else {
					_, err = updateStmt.Exec(fieldType, valInt, valDecimal, valString, valText, valDate, id, fieldName)
				}

				if err != nil {
					return nil, fmt.Errorf("failed to save custom field %s: %w", fieldName, err)
				}
			}
		}
	}

	return GetEntity(id, className, db)
}
