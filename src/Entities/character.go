package Entities

import "database/sql"

type Character struct {
	Id           int
	Name         string
	Avatar       *string
	CustomFields CustomFieldEntity
}

func GenerateCharacterTable(db *sql.DB) {
	sql := "CREATE TABLE characters (id SERIAL, name VARCHAR(255), avatar VARCHAR(255))"
	db.Exec(sql)
}
