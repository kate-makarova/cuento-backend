package Controllers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"cuento-backend/src/Entities"

	"github.com/gin-gonic/gin"
)

func GetCharacter(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	rows, err := db.Query("SELECT * FROM character_base LEFT JOIN character_flattened ON character_base.id = character_flattened.entity_id WHERE character_base.id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}
	defer rows.Close()

	if !rows.Next() {
		c.JSON(http.StatusNotFound, gin.H{"error": "Character not found"})
		return
	}

	cols, err := rows.Columns()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get columns"})
		return
	}

	vals := make([]interface{}, len(cols))
	for i := range vals {
		vals[i] = new(sql.RawBytes)
	}

	if err := rows.Scan(vals...); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan row"})
		return
	}

	var character Entities.Character
	customFieldsMap := make(map[string]interface{})

	for i, colName := range cols {
		val := vals[i].(*sql.RawBytes)

		// Handle NULL values from the database
		if *val == nil {
			continue
		}

		strVal := string(*val)

		switch colName {
		case "id":
			character.Id, _ = strconv.Atoi(strVal)
		case "name":
			character.Name = strVal
		case "avatar":
			if strVal != "" {
				character.Avatar = &strVal
			}
		case "entity_id":
			// ignore, it's the same as id
		default:
			// This is a custom field
			var v interface{}
			// Attempt to unmarshal as JSON, if it's a complex type stored as string
			if err := json.Unmarshal(*val, &v); err == nil {
				customFieldsMap[colName] = v
			} else {
				// Otherwise, just use the string value
				customFieldsMap[colName] = strVal
			}
		}
	}

	// Now, convert the map to the slice of CustomField structs
	var cfSlice []Entities.CustomField
	for name, value := range customFieldsMap {
		cfSlice = append(cfSlice, Entities.CustomField{
			FieldName:  name,
			FieldValue: value,
		})
	}
	character.CustomFields.CustomFields = cfSlice

	// I don't have the FieldConfig here. I'll leave it empty.

	c.JSON(http.StatusOK, character)
}

func CreateCharacter(c *gin.Context, db *sql.DB) {
	var character Entities.Character
	if err := c.ShouldBindJSON(&character); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Insert into character_base
	res, err := db.Exec("INSERT INTO character_base (name, avatar) VALUES (?, ?)", character.Name, character.Avatar)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create character base"})
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get character ID"})
		return
	}
	character.Id = int(id)

	// If there are custom fields, insert them
	if len(character.CustomFields.CustomFields) > 0 {
		// Get column types from character_flattened to know where to insert
		rows, err := db.Query("SELECT * FROM character_flattened WHERE 1=0")
		if err != nil {
			// If table doesn't exist, maybe no custom fields yet? Or error.
			// Assuming it exists if we are sending custom fields.
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query custom fields metadata"})
			return
		}
		defer rows.Close()

		colTypes, err := rows.ColumnTypes()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get column types"})
			return
		}

		colTypeMap := make(map[string]string)
		for _, ct := range colTypes {
			colTypeMap[ct.Name()] = ct.DatabaseTypeName()
		}

		stmt, err := db.Prepare("INSERT INTO character_main (entity_id, field_machine_name, field_type, value_int, value_decimal, value_string, value_text, value_date) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare custom field insert"})
			return
		}
		defer stmt.Close()

		for _, field := range character.CustomFields.CustomFields {
			dbType, ok := colTypeMap[field.FieldName]
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
				if v, ok := field.FieldValue.(float64); ok {
					i := int(v)
					valInt = &i
				}
			case "DECIMAL", "FLOAT", "DOUBLE":
				fieldType = "decimal"
				if v, ok := field.FieldValue.(float64); ok {
					valDecimal = &v
				}
			case "VARCHAR", "CHAR":
				fieldType = "string"
				if v, ok := field.FieldValue.(string); ok {
					valString = &v
				}
			case "TEXT", "BLOB":
				fieldType = "text"
				if v, ok := field.FieldValue.(string); ok {
					valText = &v
				}
			case "DATETIME", "DATE", "TIMESTAMP":
				fieldType = "date"
				if v, ok := field.FieldValue.(string); ok {
					valDate = &v
				}
			default:
				fieldType = "string"
				if v, ok := field.FieldValue.(string); ok {
					valString = &v
				}
			}

			_, err := stmt.Exec(id, field.FieldName, fieldType, valInt, valDecimal, valString, valText, valDate)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert custom field " + field.FieldName})
				return
			}
		}
	}

	c.JSON(http.StatusCreated, character)
}

func PatchCharacter(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var jsonMap map[string]interface{}
	if err := c.ShouldBindJSON(&jsonMap); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update character_base
	var updates []string
	var args []interface{}

	if val, ok := jsonMap["name"]; ok {
		updates = append(updates, "name = ?")
		args = append(args, val)
	}

	if val, ok := jsonMap["avatar"]; ok {
		updates = append(updates, "avatar = ?")
		args = append(args, val)
	}

	if len(updates) > 0 {
		query := "UPDATE character_base SET " + strings.Join(updates, ", ") + " WHERE id = ?"
		args = append(args, id)
		if _, err := db.Exec(query, args...); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update character base"})
			return
		}
	}

	// Update custom fields
	if cfVal, ok := jsonMap["custom_fields"]; ok {
		cfMap, ok := cfVal.(map[string]interface{})
		if ok {
			if fieldsVal, ok := cfMap["custom_fields"]; ok {
				fields, ok := fieldsVal.([]interface{})
				if ok && len(fields) > 0 {
					// Get column types
					rows, err := db.Query("SELECT * FROM character_flattened WHERE 1=0")
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query custom fields metadata"})
						return
					}
					defer rows.Close()

					colTypes, err := rows.ColumnTypes()
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get column types"})
						return
					}

					colTypeMap := make(map[string]string)
					for _, ct := range colTypes {
						colTypeMap[ct.Name()] = ct.DatabaseTypeName()
					}

					// Prepare statements
					checkStmt, err := db.Prepare("SELECT 1 FROM character_main WHERE entity_id = ? AND field_machine_name = ?")
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare check statement"})
						return
					}
					defer checkStmt.Close()

					insertStmt, err := db.Prepare("INSERT INTO character_main (entity_id, field_machine_name, field_type, value_int, value_decimal, value_string, value_text, value_date) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare insert statement"})
						return
					}
					defer insertStmt.Close()

					updateStmt, err := db.Prepare("UPDATE character_main SET field_type = ?, value_int = ?, value_decimal = ?, value_string = ?, value_text = ?, value_date = ? WHERE entity_id = ? AND field_machine_name = ?")
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare update statement"})
						return
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

						// Check if exists
						var exists int
						err := checkStmt.QueryRow(id, fieldName).Scan(&exists)
						if err != nil && err != sql.ErrNoRows {
							c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check custom field existence"})
							return
						}

						if err == sql.ErrNoRows {
							// Insert
							_, err = insertStmt.Exec(id, fieldName, fieldType, valInt, valDecimal, valString, valText, valDate)
						} else {
							// Update
							_, err = updateStmt.Exec(fieldType, valInt, valDecimal, valString, valText, valDate, id, fieldName)
						}

						if err != nil {
							c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save custom field " + fieldName})
							return
						}
					}
				}
			}
		}
	}

	// Return the updated character
	GetCharacter(c, db)
}
