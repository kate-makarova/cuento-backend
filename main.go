package main

import (
	"cuento-backend/src/Controllers"
	"cuento-backend/src/Install"
	"fmt"
	"net/http"

	"cuento-backend/src/Services"

	"github.com/gin-gonic/gin"
)

func main() {
	Services.InitDB()

	r := gin.Default()
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})
	r.GET("/install", func(c *gin.Context) {
		err := Install.ExecuteSQLFile(Services.DB, "./src/Install/default_tables.sql")
		if err != nil {
			fmt.Println(err.Error())
			return
		}
	})
	r.GET("/character/get/:id", func(c *gin.Context) {
		Controllers.GetCharacter(c, Services.DB)
	})
	r.POST("/character-template/update", func(c *gin.Context) {
		// Get the raw request body (JSON config)
		jsonData, err := c.GetRawData()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		if err := Controllers.UpdateCharacterTemplate(Services.DB, jsonData); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "updated",
		})
	})
	r.Run() // listen and serve on 0.0.0.0:8080
}
