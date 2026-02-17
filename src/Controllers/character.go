package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type CreateCharacterRequest struct {
	SubforumID   int                    `json:"subforum_id" binding:"required"`
	Name         string                 `json:"name" binding:"required"`
	Avatar       *string                `json:"avatar"`
	CustomFields map[string]interface{} `json:"custom_fields"`
	FactionIDs   []Entities.Faction     `json:"factions"`
}

func GetCharacter(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid Id"})
		c.Abort()
		return
	}

	entity, err := Services.GetEntity(int64(id), "character", db)
	if err != nil {
		if err == sql.ErrNoRows {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Character not found"})
		} else {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get character: " + err.Error()})
		}
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, entity)
}

func CreateCharacter(c *gin.Context, db *sql.DB) {
	var req CreateCharacterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	userID := Services.GetUserIdFromContext(c)
	if userID == 0 {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusUnauthorized, Message: "Unauthorized"})
		c.Abort()
		return
	}

	tx, err := db.Begin()
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to start transaction"})
		c.Abort()
		return
	}
	defer tx.Rollback()

	// Insert Topic (without first post)
	res, err := tx.Exec("INSERT INTO topics (subforum_id, name, author_user_id, date_created, date_last_post, status, type, post_number, last_post_author_user_id) VALUES (?, ?, ?, NOW(), NOW(), 0, 0, 0, ?)",
		req.SubforumID, req.Name, userID, userID)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to insert topic: " + err.Error()})
		c.Abort()
		return
	}
	topicID, err := res.LastInsertId()
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get topic ID"})
		c.Abort()
		return
	}

	if err := tx.Commit(); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to commit transaction"})
		c.Abort()
		return
	}

	character := Entities.Character{
		UserId:  userID,
		TopicId: int(topicID),
		Name:    req.Name,
		Avatar:  req.Avatar,
		CustomFields: Entities.CustomFieldEntity{
			CustomFields: req.CustomFields,
		},
	}

	createdEntity, characterID, err := Services.CreateEntity("character", &character, db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to create character: " + err.Error()})
		c.Abort()
		return
	}

	// Handle factions
	for _, faction := range req.FactionIDs {
		var factionID int

		// If faction ID is negative, create a new faction
		if faction.Id < 0 {
			newFactionID, err := Services.CreateFaction(faction, db)
			if err != nil {
				_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to create faction: " + err.Error()})
				c.Abort()
				return
			}
			factionID = int(newFactionID)
		} else {
			factionID = faction.Id
		}

		// Add faction to character
		if err := Services.AddFactionCharacter(factionID, int(characterID), db); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to add faction to character: " + err.Error()})
			c.Abort()
			return
		}
	}

	c.JSON(http.StatusCreated, createdEntity)
}

func PatchCharacter(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid Id"})
		c.Abort()
		return
	}

	var jsonMap map[string]interface{}
	if err := c.ShouldBindJSON(&jsonMap); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	updatedEntity, err := Services.PatchEntity(int64(id), "character", jsonMap, db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to patch character: " + err.Error()})
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, updatedEntity)
}

func GetCharacterList(c *gin.Context, db *sql.DB) {
	// 1. Get the faction tree
	factions, err := Services.GetFactionTree(db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get faction tree: " + err.Error()})
		c.Abort()
		return
	}

	// 2. Create a map for easy access to factions by ID
	factionMap := make(map[int]*Entities.Faction)
	for i := range factions {
		factions[i].Characters = []Entities.Character{}
		factionMap[factions[i].Id] = &factions[i]
	}

	// 3. Fetch active characters and their factions
	query := `
		WITH RankedFactions AS (
			SELECT
				c.id,
				c.name,
				c.avatar,
				f.id as faction_id,
				ROW_NUMBER() OVER(PARTITION BY c.id ORDER BY f.level DESC) as rn
			FROM
				character_base c
			JOIN
				character_faction cf ON c.id = cf.character_id
			JOIN
				factions f ON cf.faction_id = f.id
			WHERE
				c.character_status = 0
		)
		SELECT id, name, avatar, faction_id FROM RankedFactions WHERE rn = 1
	`
	rows, err := db.Query(query)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get characters: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	// 4. Assign characters to their determined faction
	for rows.Next() {
		var char Entities.Character
		var factionID int
		if err := rows.Scan(&char.Id, &char.Name, &char.Avatar, &factionID); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan character: " + err.Error()})
			c.Abort()
			return
		}
		if faction, ok := factionMap[factionID]; ok {
			faction.Characters = append(faction.Characters, char)
		}
	}

	c.JSON(http.StatusOK, factions)
}

func GetCharacterAutocomplete(c *gin.Context, db *sql.DB) {
	query := `
		SELECT id, name FROM character_base WHERE name LIKE ? ORDER BY name ASC LIMIT 10
	`
	rows, err := db.Query(query, "%"+c.Param("term")+"%")
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get characters: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()
	var characters []Entities.ShortCharacter
	for rows.Next() {
		var tempCharacter Entities.ShortCharacter
		if err := rows.Scan(&tempCharacter.Id, &tempCharacter.Name); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan character: " + err.Error()})
		}
		characters = append(characters, tempCharacter)
	}
	c.JSON(http.StatusOK, characters)
}

func GetUserCharacters(c *gin.Context, db *sql.DB) {
	userID := Services.GetUserIdFromContext(c)
	if userID == 0 {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusUnauthorized, Message: "Unauthorized"})
		c.Abort()
		return
	}

	query := "SELECT id, name FROM character_base WHERE user_id = ?"
	rows, err := db.Query(query, userID)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get user characters: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	var characters []Entities.ShortCharacter
	for rows.Next() {
		var char Entities.ShortCharacter
		if err := rows.Scan(&char.Id, &char.Name); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan character: " + err.Error()})
			c.Abort()
			return
		}
		characters = append(characters, char)
	}

	c.JSON(http.StatusOK, characters)
}

func GetCharacterProfilesByUser(c *gin.Context, db *sql.DB) {
	userID := Services.GetUserIdFromContext(c)
	if userID == 0 {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusUnauthorized, Message: "Unauthorized"})
		c.Abort()
		return
	}

	// Get IDs of profiles for this user
	query := "SELECT cp.id, cp.character_id, cb.name, cp.avatar FROM character_profile_base cp JOIN character_base cb ON cp.character_id = cb.id WHERE cb.user_id = ?"
	rows, err := db.Query(query, userID)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get character profiles: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	var profiles []Entities.CharacterProfile
	for rows.Next() {
		var id int
		var characterId int
		var name string
		var avatar *string
		if err := rows.Scan(&id, &characterId, &name, &avatar); err != nil {
			continue
		}

		entity, err := Services.GetEntity(int64(id), "character_profile", db)
		if err != nil {
			continue
		}

		if profile, ok := entity.(*Entities.CharacterProfile); ok {
			profile.CharacterId = characterId
			profile.CharacterName = name
			profile.Avatar = avatar
			profiles = append(profiles, *profile)
		}
	}

	c.JSON(http.StatusOK, profiles)
}
