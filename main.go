package main

import (
	"cuento-backend/src/Controllers"
	"cuento-backend/src/Install"
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {
	Services.InitDB()

	r := gin.Default()

	// Public routes
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

	// User routes (Public)
	r.POST("/register", func(c *gin.Context) {
		Controllers.Register(c, Services.DB)
	})
	r.POST("/login", func(c *gin.Context) {
		Controllers.Login(c, Services.DB)
	})

	// Protected routes
	protected := r.Group("/")
	protected.Use(Middlewares.AuthMiddleware())
	{
		protected.GET("/character/get/:id", func(c *gin.Context) {
			Controllers.GetCharacter(c, Services.DB)
		})
		protected.POST("/character/create", func(c *gin.Context) {
			Controllers.CreateCharacter(c, Services.DB)
		})
		protected.PATCH("/character/update/:id", func(c *gin.Context) {
			Controllers.PatchCharacter(c, Services.DB)
		})

		protected.POST("/character-template/update", func(c *gin.Context) {
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
	}

	r.Run() // listen and serve on 0.0.0.0:8080
}
