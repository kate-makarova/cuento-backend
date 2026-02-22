package Services

import (
	"cuento-backend/src/Entities"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func GetPostById(id int, db *sql.DB) (*Entities.Post, error) {
	// 1. Get custom field columns from the config table
	var configJSON string
	err := db.QueryRow("SELECT config FROM custom_field_config WHERE entity_type = 'character_profile'").Scan(&configJSON)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get character profile config: %w", err)
	}

	var customConfig []Entities.CustomFieldConfig
	if configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &customConfig); err != nil {
			return nil, fmt.Errorf("failed to parse character profile config: %w", err)
		}
	}

	var flattenedCols []string
	for _, field := range customConfig {
		flattenedCols = append(flattenedCols, "cpf."+field.MachineFieldName)
	}

	colsSelect := ""
	if len(flattenedCols) > 0 {
		colsSelect = ", " + strings.Join(flattenedCols, ", ")
	}

	// 2. Construct the main query
	query := fmt.Sprintf(`
		SELECT
			p.id, p.author_user_id, p.date_created, p.content, p.use_character_profile,
			u.username, u.avatar,
			cp.id as character_profile_id, cp.character_id, cb.name as character_name, cp.avatar as character_avatar
			%s
		FROM posts p
		LEFT JOIN users u ON p.author_user_id = u.id
		LEFT JOIN character_profile_base cp ON p.character_profile_id = cp.id
		LEFT JOIN character_base cb ON cp.character_id = cb.id
		LEFT JOIN character_profile_flattened cpf ON cp.id = cpf.entity_id
		WHERE p.id = ?
	`, colsSelect)

	// 3. Scan and process results
	rows, err := db.Query(query, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get post: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, sql.ErrNoRows
	}

	cols, _ := rows.Columns()
	values := make([]interface{}, len(cols))
	for i := range values {
		values[i] = new(sql.RawBytes)
	}

	if err := rows.Scan(values...); err != nil {
		return nil, fmt.Errorf("failed to scan post data: %w", err)
	}

	rowMap := make(map[string]interface{})
	for i, colName := range cols {
		val := *(values[i].(*sql.RawBytes))
		if val != nil {
			rowMap[colName] = string(val)
		}
	}

	var post Entities.Post
	if val, ok := rowMap["id"]; ok {
		post.Id, _ = strconv.Atoi(val.(string))
	}
	if val, ok := rowMap["author_user_id"]; ok {
		post.AuthorUserId, _ = strconv.Atoi(val.(string))
	}
	if val, ok := rowMap["date_created"]; ok {
		post.DateCreated, _ = time.Parse("2006-01-02 15:04:05", val.(string))
	}
	if val, ok := rowMap["content"]; ok {
		post.Content = val.(string)
		post.ContentHtml = Entities.ParseBBCode(post.Content)
	}
	if val, ok := rowMap["use_character_profile"]; ok {
		post.UseCharacterProfile, _ = strconv.ParseBool(val.(string))
	}

	if post.UseCharacterProfile {
		var charProfile Entities.CharacterProfile
		if id, ok := rowMap["character_profile_id"]; ok {
			charProfile.Id, _ = strconv.Atoi(id.(string))
		}
		if id, ok := rowMap["character_id"]; ok {
			charProfile.CharacterId, _ = strconv.Atoi(id.(string))
		}
		if name, ok := rowMap["character_name"]; ok {
			charProfile.CharacterName = name.(string)
		}
		if avatar, ok := rowMap["character_avatar"]; ok {
			avatarStr := avatar.(string)
			charProfile.Avatar = &avatarStr
		}

		customFields := make(map[string]Entities.CustomFieldValue)
		for _, field := range customConfig {
			if val, ok := rowMap[field.MachineFieldName]; ok {
				cfValue := Entities.CustomFieldValue{Content: val}
				if field.FieldType == "text" {
					if s, ok := val.(string); ok {
						cfValue.ContentHtml = Entities.ParseBBCode(s)
					}
				}
				customFields[field.MachineFieldName] = cfValue
			}
		}
		charProfile.CustomFields.CustomFields = customFields
		charProfile.CustomFields.FieldConfig = customConfig
		post.CharacterProfile = &charProfile
	} else {
		var userProfile Entities.UserProfile
		userProfile.UserId = post.AuthorUserId
		if username, ok := rowMap["username"]; ok {
			userProfile.UserName = username.(string)
		}
		if avatar, ok := rowMap["avatar"]; ok {
			userProfile.Avatar = avatar.(string)
		}
		post.UserProfile = &userProfile
	}

	return &post, nil
}
