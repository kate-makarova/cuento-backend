package Controllers

import (
	"cuento-backend/src/Entities"
	"database/sql"
	"encoding/json"
)

func GetCharacterTemplate(db *sql.DB) (string, error) {
	var config string
	err := db.QueryRow("SELECT config FROM custom_field_config WHERE entity_type = 'character'").Scan(&config)
	if err != nil {
		if err == sql.ErrNoRows {
			return "{}", nil
		}
		return "", err
	}
	return config, nil
}

func UpdateCharacterTemplate(db *sql.DB, config string) error {
	_, err := db.Exec("UPDATE custom_field_config SET config = ? WHERE entity_type = 'character'", config)
	if err != nil {
		return err
	}

	// Check if the flattened table exists using a reliable query
	var tableExists int
	err = db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", "character_flattened").Scan(&tableExists)
	if err != nil {
		return err
	}

	var customConfig []Entities.CustomFieldConfig
	err = json.Unmarshal([]byte(config), &customConfig)
	if err != nil {
		return err
	}
	customFieldEntity := Entities.CustomFieldEntity{FieldConfig: customConfig}

	if tableExists == 0 {
		return Entities.GenerateEntityTables(customFieldEntity, "character", db)
	}

	return Entities.UpdateFlattenedTable(customFieldEntity, "character", db)
}
