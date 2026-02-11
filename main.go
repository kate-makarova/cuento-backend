package main

import (
	"cuento-backend/src/Controllers"
	"cuento-backend/src/Install"
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"fmt"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	Services.InitDB()

	r := gin.Default()
	r.Use(cors.Default())

	// Apply error middleware globally
	r.Use(Middlewares.ErrorMiddleware())

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
	r.GET("/board/info", func(c *gin.Context) {
		Controllers.GetBoard(c, Services.DB)
	})
	r.GET("/categories/home", func(c *gin.Context) {
		Controllers.GetHomeCategories(c, Services.DB)
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

		// Character Template routes
		protected.GET("/character-template/get", func(c *gin.Context) {
			Controllers.GetCharacterTemplate(c, Services.DB)
		})
		protected.POST("/character-template/update", func(c *gin.Context) {
			Controllers.UpdateCharacterTemplate(c, Services.DB)
		})
	}

	r.Run() // listen and serve on 0.0.0.0:8080
}
