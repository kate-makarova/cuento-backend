package Features

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Feature interface {
	Name() string
	Description() string
	Key() string
	IsActive() bool
}

type Features struct {
}

func (Features) Get() []Feature {
	return []Feature{
		CurrencyFeature{},
		PostTopFeature{},
		GameDigestFeature{},
	}
}

func (Features) IsActive(feature Feature) bool {
	return feature.IsActive()
}

type featureResponse struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    bool   `json:"is_active"`
}

func ToggleFeatureHandler(c *gin.Context, db *sql.DB) {
	key := c.Param("key")
	var req struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}
	if _, err := db.Exec("UPDATE features SET is_active = ? WHERE `key` = ?", req.IsActive, key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle feature"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"key": key, "is_active": req.IsActive})
}

func GetFeaturesHandler(c *gin.Context) {
	flagMap, _ := c.Get("features")
	flags, _ := flagMap.(map[string]bool)

	all := Features{}.Get()
	result := make([]featureResponse, 0, len(all))
	for _, f := range all {
		result = append(result, featureResponse{
			Key:         f.Key(),
			Name:        f.Name(),
			Description: f.Description(),
			IsActive:    flags[f.Key()],
		})
	}

	c.JSON(http.StatusOK, result)
}
