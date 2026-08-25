package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type CreateCampaignRequest struct {
	Title          string     `json:"title" binding:"required"`
	Summary        string     `json:"summary" binding:"required"`
	Status         *int       `json:"status"`
	StartDate      *time.Time `json:"start_date"`
	EndDate        *time.Time `json:"end_date"`
	CharacterIDs   []int      `json:"character_ids"`
	EpisodeIDs     []int      `json:"episode_ids"`
	NpcCharacterIDs []int     `json:"npc_character_ids"`
	GameMasterIDs  []int      `json:"game_master_ids"`
}

type UpdateCampaignRequest struct {
	Title          string     `json:"title" binding:"required"`
	Summary        string     `json:"summary" binding:"required"`
	Status         *int       `json:"status"`
	StartDate      *time.Time `json:"start_date"`
	EndDate        *time.Time `json:"end_date"`
	CharacterIDs   []int      `json:"character_ids"`
	EpisodeIDs     []int      `json:"episode_ids"`
	NpcCharacterIDs []int     `json:"npc_character_ids"`
	GameMasterIDs  []int      `json:"game_master_ids"`
}

func loadCampaignRelations(campaign *Entities.Campaign, db *sql.DB) {
	campaign.Characters = []Entities.ShortCharacter{}
	campaign.Episodes = []Entities.Episode{}
	campaign.NpcCharacters = []Entities.NpcCharacter{}
	campaign.GameMasters = []Entities.ShortUser{}

	charRows, err := db.Query(`
		SELECT cb.id, cb.name, cb.avatar
		FROM campaign_characters cc
		JOIN character_base cb ON cb.id = cc.character_id
		WHERE cc.campaign_id = ?`, campaign.Id)
	if err == nil {
		defer charRows.Close()
		for charRows.Next() {
			var ch Entities.ShortCharacter
			if charRows.Scan(&ch.Id, &ch.Name, &ch.Avatar) == nil {
				campaign.Characters = append(campaign.Characters, ch)
			}
		}
	}

	epRows, err := db.Query(`
		SELECT eb.id, eb.name
		FROM campaign_episodes ce
		JOIN episode_base eb ON eb.id = ce.episode_id
		WHERE ce.campaign_id = ?`, campaign.Id)
	if err == nil {
		defer epRows.Close()
		for epRows.Next() {
			var ep Entities.Episode
			if epRows.Scan(&ep.Id, &ep.Name) == nil {
				campaign.Episodes = append(campaign.Episodes, ep)
			}
		}
	}

	npcRows, err := db.Query(`
		SELECT nb.id, nb.name, nb.avatar
		FROM campaign_npc_characters cnc
		JOIN npc_character_base nb ON nb.id = cnc.npc_character_id
		WHERE cnc.campaign_id = ?`, campaign.Id)
	if err == nil {
		defer npcRows.Close()
		for npcRows.Next() {
			var npc Entities.NpcCharacter
			if npcRows.Scan(&npc.Id, &npc.Name, &npc.Avatar) == nil {
				campaign.NpcCharacters = append(campaign.NpcCharacters, npc)
			}
		}
	}

	gmRows, err := db.Query(`
		SELECT u.id, u.username
		FROM campaign_game_masters cgm
		JOIN users u ON u.id = cgm.user_id
		WHERE cgm.campaign_id = ?`, campaign.Id)
	if err == nil {
		defer gmRows.Close()
		for gmRows.Next() {
			var gm Entities.ShortUser
			if gmRows.Scan(&gm.Id, &gm.Username) == nil {
				campaign.GameMasters = append(campaign.GameMasters, gm)
			}
		}
	}
}

func syncCampaignRelations(campaignID int, req struct {
	CharacterIDs    []int
	EpisodeIDs      []int
	NpcCharacterIDs []int
	GameMasterIDs   []int
}, tx *sql.Tx) error {
	if _, err := tx.Exec("DELETE FROM campaign_characters WHERE campaign_id = ?", campaignID); err != nil {
		return err
	}
	for _, id := range req.CharacterIDs {
		if _, err := tx.Exec("INSERT INTO campaign_characters (campaign_id, character_id) VALUES (?, ?)", campaignID, id); err != nil {
			return err
		}
	}

	if _, err := tx.Exec("DELETE FROM campaign_episodes WHERE campaign_id = ?", campaignID); err != nil {
		return err
	}
	for _, id := range req.EpisodeIDs {
		if _, err := tx.Exec("INSERT INTO campaign_episodes (campaign_id, episode_id) VALUES (?, ?)", campaignID, id); err != nil {
			return err
		}
	}

	if _, err := tx.Exec("DELETE FROM campaign_npc_characters WHERE campaign_id = ?", campaignID); err != nil {
		return err
	}
	for _, id := range req.NpcCharacterIDs {
		if _, err := tx.Exec("INSERT INTO campaign_npc_characters (campaign_id, npc_character_id) VALUES (?, ?)", campaignID, id); err != nil {
			return err
		}
	}

	if _, err := tx.Exec("DELETE FROM campaign_game_masters WHERE campaign_id = ?", campaignID); err != nil {
		return err
	}
	for _, id := range req.GameMasterIDs {
		if _, err := tx.Exec("INSERT INTO campaign_game_masters (campaign_id, user_id) VALUES (?, ?)", campaignID, id); err != nil {
			return err
		}
	}

	return nil
}

func CreateCampaign(c *gin.Context, db *sql.DB) {
	var req CreateCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
		c.Abort()
		return
	}

	status := Entities.PendingCampaign
	if req.Status != nil {
		status = Entities.CampaignStatus(*req.Status)
	}

	tx, err := db.Begin()
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to start transaction"})
		c.Abort()
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		"INSERT INTO campaigns (title, summary, status, start_date, end_date) VALUES (?, ?, ?, ?, ?)",
		req.Title, req.Summary, status, req.StartDate, req.EndDate,
	)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to create campaign: " + err.Error()})
		c.Abort()
		return
	}

	campaignID, _ := res.LastInsertId()

	if err := syncCampaignRelations(int(campaignID), struct {
		CharacterIDs    []int
		EpisodeIDs      []int
		NpcCharacterIDs []int
		GameMasterIDs   []int
	}{req.CharacterIDs, req.EpisodeIDs, req.NpcCharacterIDs, req.GameMasterIDs}, tx); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to save campaign relations: " + err.Error()})
		c.Abort()
		return
	}

	if err := tx.Commit(); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to commit transaction"})
		c.Abort()
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Campaign created successfully", "campaign_id": campaignID})
}

func GetCampaign(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid campaign ID"})
		c.Abort()
		return
	}

	var campaign Entities.Campaign
	err = db.QueryRow(
		"SELECT id, title, summary, status, date_created, start_date, end_date FROM campaigns WHERE id = ?", id,
	).Scan(&campaign.Id, &campaign.Title, &campaign.Summary, &campaign.Status, &campaign.DateCreated, &campaign.StartDate, &campaign.EndDate)
	if err == sql.ErrNoRows {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Campaign not found"})
		c.Abort()
		return
	}
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get campaign: " + err.Error()})
		c.Abort()
		return
	}

	loadCampaignRelations(&campaign, db)

	c.JSON(http.StatusOK, campaign)
}

func GetCampaignList(c *gin.Context, db *sql.DB) {
	rows, err := db.Query("SELECT id, title, summary, status, date_created, start_date, end_date FROM campaigns ORDER BY date_created DESC")
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to get campaigns: " + err.Error()})
		c.Abort()
		return
	}
	defer rows.Close()

	campaigns := []Entities.Campaign{}
	for rows.Next() {
		var campaign Entities.Campaign
		if err := rows.Scan(&campaign.Id, &campaign.Title, &campaign.Summary, &campaign.Status, &campaign.DateCreated, &campaign.StartDate, &campaign.EndDate); err != nil {
			_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to scan campaign: " + err.Error()})
			c.Abort()
			return
		}
		campaign.Characters = []Entities.ShortCharacter{}
		campaign.Episodes = []Entities.Episode{}
		campaign.NpcCharacters = []Entities.NpcCharacter{}
		campaign.GameMasters = []Entities.ShortUser{}
		campaigns = append(campaigns, campaign)
	}

	c.JSON(http.StatusOK, campaigns)
}

func UpdateCampaign(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid campaign ID"})
		c.Abort()
		return
	}

	var req UpdateCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid request body: " + err.Error()})
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

	res, err := tx.Exec(
		"UPDATE campaigns SET title = ?, summary = ?, status = ?, start_date = ?, end_date = ? WHERE id = ?",
		req.Title, req.Summary, req.Status, req.StartDate, req.EndDate, id,
	)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to update campaign: " + err.Error()})
		c.Abort()
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Campaign not found"})
		c.Abort()
		return
	}

	if err := syncCampaignRelations(id, struct {
		CharacterIDs    []int
		EpisodeIDs      []int
		NpcCharacterIDs []int
		GameMasterIDs   []int
	}{req.CharacterIDs, req.EpisodeIDs, req.NpcCharacterIDs, req.GameMasterIDs}, tx); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to update campaign relations: " + err.Error()})
		c.Abort()
		return
	}

	if err := tx.Commit(); err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to commit transaction"})
		c.Abort()
		return
	}

	var campaign Entities.Campaign
	_ = db.QueryRow(
		"SELECT id, title, summary, status, date_created, start_date, end_date FROM campaigns WHERE id = ?", id,
	).Scan(&campaign.Id, &campaign.Title, &campaign.Summary, &campaign.Status, &campaign.DateCreated, &campaign.StartDate, &campaign.EndDate)
	loadCampaignRelations(&campaign, db)

	c.JSON(http.StatusOK, campaign)
}

func DeleteCampaign(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusBadRequest, Message: "Invalid campaign ID"})
		c.Abort()
		return
	}

	res, err := db.Exec("DELETE FROM campaigns WHERE id = ?", id)
	if err != nil {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusInternalServerError, Message: "Failed to delete campaign: " + err.Error()})
		c.Abort()
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		_ = c.Error(&Middlewares.AppError{Code: http.StatusNotFound, Message: "Campaign not found"})
		c.Abort()
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Campaign deleted successfully"})
}
