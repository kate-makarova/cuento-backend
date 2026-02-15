package main

import (
	"cuento-backend/src/Controllers"
	"cuento-backend/src/Install"
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"cuento-backend/src/Websockets"
	"fmt"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	Services.InitDB()
	Services.RegisterEventHandlers(Services.DB)

	// Start WebSocket Hub
	go Websockets.MainHub.Run()

	r := gin.Default()
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	r.Use(cors.New(config))

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
	r.GET("/viewforum/:subforum/:page", func(c *gin.Context) {
		Controllers.GetTopicsBySubforum(c, Services.DB)
	})
	r.GET("/viewtopic/:id/:page", func(c *gin.Context) {
		Controllers.GetPostsByTopic(c, Services.DB)
	})
	r.GET("/character-list", func(c *gin.Context) {
		Controllers.GetCharacterList(c, Services.DB)
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
		protected.GET("/faction-children/:parent_id/get", func(c *gin.Context) {
			Controllers.GetFactionChildren(c, Services.DB)
		})

		// Character Template routes
		protected.GET("/template/:type/get", func(c *gin.Context) {
			Controllers.GetTemplate(c, Services.DB)
		})
		protected.POST("/template/:type/update", func(c *gin.Context) {
			Controllers.UpdateTemplate(c, Services.DB)
		})
		protected.POST("/episode/create", func(c *gin.Context) {
			Controllers.CreateEpisode(c, Services.DB)
		})
		protected.GET("/ws", func(c *gin.Context) {
			Controllers.HandleWebSocket(c)
		})
	}

	r.Run() // listen and serve on 0.0.0.0:8080
}
