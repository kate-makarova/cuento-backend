package Entities

import "database/sql"

type Character struct {
	Id           int               `json:"id"`
	Name         string            `json:"name"`
	Avatar       *string           `json:"avatar"`
	CustomFields CustomFieldEntity `json:"custom_fields"`
}

func GenerateCharacterTable(db *sql.DB) {
	sql := "CREATE TABLE character_base (id SERIAL, name VARCHAR(255), avatar VARCHAR(255))"
	db.Exec(sql)
}
