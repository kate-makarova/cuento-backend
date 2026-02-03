package Controllers

import "database/sql"

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
