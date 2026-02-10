package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Services"
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func GetCharacter(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	entity, err := Services.GetEntity(int64(id), "character", db)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Character not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get character: " + err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, entity)
}

func CreateCharacter(c *gin.Context, db *sql.DB) {
	var character Entities.Character
	if err := c.ShouldBindJSON(&character); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	createdEntity, err := Services.CreateEntity("character", &character, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create character: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, createdEntity)
}

func PatchCharacter(c *gin.Context, db *sql.DB) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var jsonMap map[string]interface{}
	if err := c.ShouldBindJSON(&jsonMap); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updatedEntity, err := Services.PatchEntity(int64(id), "character", jsonMap, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to patch character: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, updatedEntity)
}
