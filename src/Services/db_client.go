package Services

import (
	"database/sql"
	"log"
	"time"

	"cuento-backend/config"

	_ "github.com/go-sql-driver/mysql"
)

var DB *sql.DB

func InitDB() {
	cfg := config.LoadDBConfig()
	dsn := cfg.DSN()

	var err error
	DB, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}

	// Retry connection for Docker startup timing
	for i := 0; i < 10; i++ {
		err = DB.Ping()
		if err == nil {
			break
		}
		log.Printf("Database not ready, retrying... (%d/10)", i+1)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Fatalf("Could not connect to database: %v", err)
	}

	log.Println("Successfully connected to the database")
}
