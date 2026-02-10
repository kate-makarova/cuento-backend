package Entities

import "database/sql"

type CharacterProfile struct {
	Id           int               `json:"id"`
	CharacterId  int               `json:"character_id"`
	CustomFields CustomFieldEntity `json:"custom_fields"`
}

func GenerateCharacterProfileTable(db *sql.DB) {
	sql := "create table character_base" +
		"(id      bigint unsigned auto_increment," +
		"character_id int          null," +
		"constraint id" +
		"unique (id)," +
		"constraint character_profile_base_character_id_fk" +
		"foreign key (character_id) references character_base (id));"
	db.Exec(sql)
}
