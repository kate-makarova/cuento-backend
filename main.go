package main

import (
	"cuento-backend/src/Controllers"
	"cuento-backend/src/Install"
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Router"
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
	publicRouter := Router.NewCustomRouter(r.Group("/"))

	// User routes (Public)
	publicRouter.POST("/register", "Register a new user account", func(c *gin.Context) {
		Controllers.Register(c, Services.DB)
	})
	publicRouter.POST("/login", "Login with user credentials", func(c *gin.Context) {
		Controllers.Login(c, Services.DB)
	})
	publicRouter.GET("/board/info", "Get board information", func(c *gin.Context) {
		Controllers.GetBoard(c, Services.DB)
	})

	// Optional Auth routes (Context populated if token present, otherwise Guest)
	optionalAuthGroup := r.Group("/")
	optionalAuthGroup.Use(Middlewares.OptionalAuthMiddleware())
	optionalAuthRouter := Router.NewCustomRouter(optionalAuthGroup)
	optionalAuthRouter.GET("/categories/home", "Get home page categories", func(c *gin.Context) {
		Controllers.GetHomeCategories(c, Services.DB)
	})
	optionalAuthRouter.GET("/ping", "Health check endpoint", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})
	optionalAuthRouter.GET("/install", "Install default database tables", func(c *gin.Context) {
		err := Install.ExecuteSQLFile(Services.DB, "./src/Install/default_tables.sql")
		if err != nil {
			fmt.Println(err.Error())
			return
		}
	})
	optionalAuthRouter.GET("/viewforum/:subforum/:page", "Get topics in a subforum by page", func(c *gin.Context) {
		Controllers.GetTopicsBySubforum(c, Services.DB)
	})
	optionalAuthRouter.GET("/viewtopic/:id/:page", "Get posts in a topic by page", func(c *gin.Context) {
		Controllers.GetPostsByTopic(c, Services.DB)
	})
	optionalAuthRouter.GET("/character-list", "Get list of all characters", func(c *gin.Context) {
		Controllers.GetCharacterList(c, Services.DB)
	})
	optionalAuthRouter.GET("/subforum/list-short", "Get list of all subforums", func(c *gin.Context) {
		Controllers.GetShortSubforumList(c, Services.DB)
	})
	optionalAuthRouter.GET("/character-autocomplete/:term", "Get list of characters matching search term", func(c *gin.Context) {
		Controllers.GetCharacterAutocomplete(c, Services.DB)
	})
	optionalAuthRouter.GET("/factions/get", "Get faction tree", func(c *gin.Context) {
		Controllers.GetFactionTree(c, Services.DB)
	})
	optionalAuthRouter.POST("/episodes/get", "Get episode list", func(c *gin.Context) {
		Controllers.GetEpisodes(c, Services.DB)
	})
	optionalAuthRouter.GET("/subforum/get/:id", "Get subforum details by ID", func(c *gin.Context) {
		Controllers.GetSubforum(c, Services.DB)
	})

	// Protected routes
	protectedGroup := r.Group("/")
	protectedGroup.Use(Middlewares.AuthMiddleware())
	protectedGroup.Use(Middlewares.PermissionsMiddleware(Services.DB))
	protectedRouter := Router.NewCustomRouter(protectedGroup)

	protectedRouter.GET("/character/get/:id", "Get character details by ID", func(c *gin.Context) {
		Controllers.GetCharacter(c, Services.DB)
	})
	protectedRouter.POST("/character/create", "Create a new character", func(c *gin.Context) {
		Controllers.CreateCharacter(c, Services.DB)
	})
	protectedRouter.PATCH("/character/update/:id", "Update character by ID", func(c *gin.Context) {
		Controllers.PatchCharacter(c, Services.DB)
	})
	protectedRouter.GET("/user/characters", "Get current user's characters", func(c *gin.Context) {
		Controllers.GetUserCharacters(c, Services.DB)
	})
	protectedRouter.GET("/faction-children/:parent_id/get", "Get child factions by parent ID", func(c *gin.Context) {
		Controllers.GetFactionChildren(c, Services.DB)
	})

	// Character Template routes
	protectedRouter.GET("/template/:type/get", "Get character template by type", func(c *gin.Context) {
		Controllers.GetTemplate(c, Services.DB)
	})
	protectedRouter.POST("/template/:type/update", "Update character template by type", func(c *gin.Context) {
		Controllers.UpdateTemplate(c, Services.DB)
	})
	protectedRouter.POST("/episode/create", "Create a new episode", func(c *gin.Context) {
		Controllers.CreateEpisode(c, Services.DB)
	})
	protectedRouter.GET("/ws", "WebSocket connection endpoint", func(c *gin.Context) {
		Controllers.HandleWebSocket(c)
	})

	r.Run() // listen and serve on 0.0.0.0:8080
}
