package Entities

import "database/sql"

type Subform struct {
	Id          int    `json:"int"`
	Name        string `json:"name"`
	Position    int    `json:"position"`
	TopicNumber int    `json:"topic_number"`
	PostNumber  int    `json:"post_number"`
}

func GenerateSubformTable(db *sql.DB) {
	sql := "CREATE TABLE IF NOT EXISTS subform_base (" +
		"id BIGINT UNSIGNED AUTO_INCREMENT," +
		"name VARCHAR(255) NULL," +
		"position INT NULL," +
		"topic_number INT NULL," +
		"post_number INT NULL," +
		"CONSTRAINT id UNIQUE (id)" +
		");"
	db.Exec(sql)
}
