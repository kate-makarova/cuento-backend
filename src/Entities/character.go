package Entities

import "database/sql"

type Character struct {
	Id           int               `json:"id"`
	UserId       int               `json:"user_id"`
	Name         string            `json:"name"`
	Avatar       *string           `json:"avatar"`
	CustomFields CustomFieldEntity `json:"custom_fields"`
}

func GenerateCharacterTable(db *sql.DB) {
	sql := "create table character_base" +
		"(id      bigint unsigned auto_increment," +
		"user_id int          null," +
		"name    varchar(255) null," +
		"avatar  varchar(255) null," +
		"constraint id" +
		"unique (id)," +
		"constraint character_base_users_id_fk" +
		"foreign key (user_id) references users (id));"
	db.Exec(sql)
}
