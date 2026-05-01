package config

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func ConnectDB() {
	var err error
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		log.Fatal("Database URL not found in .env")
	}

	DB, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Fail to initiate DB", err)
	}

	if err = DB.Ping(); err != nil {
		log.Fatal("Fail to connect to db", err)
	}

	log.Println("Connected to DB")
}
