package main

import (
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
	r.Run() // listen and serve on 0.0.0.0:8080
}
