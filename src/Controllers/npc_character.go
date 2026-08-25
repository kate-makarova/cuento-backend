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

type CreateNpcCharacterRequest struct {
	Name         string                 `json:"name" binding:"required"`
	Avatar       *string                `json:"avatar"`
	CampaignId   *int                   `json:"campaign_id"`
	CustomFields map[string]interface{} `json:"custom_fields"`
}

type UpdateNpcCharacterRequest struct {
	Name         string                 `json:"name" binding:"required"`
	Avatar       *string                `json:"avatar"`
	CampaignId   *int                   `json:"campaign_id"`
	CustomFields map[string]interface{} `json:"custom_fields"`
}

func getNpcCharacter(id int, db *sql.DB) (*Entities.NpcCharacter, error) {
	var npc Entities.NpcCharacter
	npc.CustomFields = Entities.CustomFieldEntity{
		CustomFields: map[string]Entities.CustomFieldValue{},
		FieldConfig:  []Entities.CustomFieldConfig{},
	}
	err := db.QueryRow(
		"SELECT id, name, avatar, campaign_id FROM npc_character_base WHERE id = ?", id,
	).Scan(&npc.Id, &npc.Name, &npc.Avatar, &npc.CampaignId)
	if err != nil {
		return nil, err
	}
	return &npc, nil
}

func checkNpcEditPermission(userID int, npc *Entities.NpcCharacter, db *sql.DB) (bool, error) {
	if npc.CampaignId != nil {
		return Services.IsGameMasterOfCampaign(userID, *npc.CampaignId, db)
	}
	return Services.HasPermission(userID, "npc_character_edit", db)
}

func GetNpcCharacterList(c *gin.Context, db *sql.DB) {
	query := "SELECT id, name, avatar, campaign_id FROM npc_character_base"
	var args []interface{}

	if campaignIDStr := c.Query("campaign_id"); campaignIDStr != "" {
		campaignID, err := strconv.Atoi(campaignIDStr)
		if err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid campaign_id"})
			c.Abort()
			return
		}
		query += " WHERE campaign_id = ?"
		args = append(args, campaignID)
	}

	query += " ORDER BY name ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get NPC characters: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	result := []Entities.NpcCharacter{}
	for rows.Next() {
		var npc Entities.NpcCharacter
		npc.CustomFields = Entities.CustomFieldEntity{
			CustomFields: map[string]Entities.CustomFieldValue{},
			FieldConfig:  []Entities.CustomFieldConfig{},
		}
		if err := rows.Scan(&npc.Id, &npc.Name, &npc.Avatar, &npc.CampaignId); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan NPC character: " + err.Error()})
			c.Abort()
			return
		}
		result = append(result, npc)
	}

	c.JSON(http.StatusOK, result)
}

func GetNpcCharacter(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid NPC character ID"})
		c.Abort()
		return
	}

	npc, err := getNpcCharacter(id, db)
	if err == sql.ErrNoRows {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "NPC character not found"})
		c.Abort()
		return
	}
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get NPC character: " + err.Error()})
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, npc)
}

func CreateNpcCharacter(c *gin.Context, db *sql.DB) {
	var req CreateNpcCharacterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	userID := Services.GetUserIdFromContext(c)

	if req.CampaignId != nil {
		isGM, err := Services.IsGameMasterOfCampaign(userID, *req.CampaignId, db)
		if err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to check campaign permissions: " + err.Error()})
			c.Abort()
			return
		}
		if !isGM {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusForbidden, Message: "You are not a game master of this campaign"})
			c.Abort()
			return
		}
	}

	res, err := db.Exec(
		"INSERT INTO npc_character_base (name, avatar, campaign_id) VALUES (?, ?, ?)",
		req.Name, req.Avatar, req.CampaignId,
	)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to create NPC character: " + err.Error()})
		c.Abort()
		return
	}

	id, _ := res.LastInsertId()
	c.JSON(http.StatusCreated, gin.H{"message": "NPC character created successfully", "npc_character_id": id})
}

func UpdateNpcCharacter(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid NPC character ID"})
		c.Abort()
		return
	}

	var req UpdateNpcCharacterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	npc, err := getNpcCharacter(id, db)
	if err == sql.ErrNoRows {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "NPC character not found"})
		c.Abort()
		return
	}
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to fetch NPC character: " + err.Error()})
		c.Abort()
		return
	}

	userID := Services.GetUserIdFromContext(c)
	canEdit, err := checkNpcEditPermission(userID, npc, db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to check permissions: " + err.Error()})
		c.Abort()
		return
	}
	if !canEdit {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusForbidden, Message: "You do not have permission to edit this NPC character"})
		c.Abort()
		return
	}

	_, err = db.Exec(
		"UPDATE npc_character_base SET name = ?, avatar = ?, campaign_id = ? WHERE id = ?",
		req.Name, req.Avatar, req.CampaignId, id,
	)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to update NPC character: " + err.Error()})
		c.Abort()
		return
	}

	npc, _ = getNpcCharacter(id, db)
	c.JSON(http.StatusOK, npc)
}

func DeleteNpcCharacter(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid NPC character ID"})
		c.Abort()
		return
	}

	npc, err := getNpcCharacter(id, db)
	if err == sql.ErrNoRows {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "NPC character not found"})
		c.Abort()
		return
	}
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to fetch NPC character: " + err.Error()})
		c.Abort()
		return
	}

	userID := Services.GetUserIdFromContext(c)
	canEdit, err := checkNpcEditPermission(userID, npc, db)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to check permissions: " + err.Error()})
		c.Abort()
		return
	}
	if !canEdit {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusForbidden, Message: "You do not have permission to delete this NPC character"})
		c.Abort()
		return
	}

	_, err = db.Exec("DELETE FROM npc_character_base WHERE id = ?", id)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to delete NPC character: " + err.Error()})
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "NPC character deleted successfully"})
}
