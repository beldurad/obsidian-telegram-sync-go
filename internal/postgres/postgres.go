package postgres

import (
	"database/sql"
	"log"
	"os"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/config"

	"github.com/lib/pq"
)

func New(dbCfg config.DatabaseConfig) *sql.DB {
	cfg := pq.Config{
		Host:     dbCfg.Host,
		Port:     dbCfg.Port,
		User:     dbCfg.User,
		Password: dbCfg.Password,
		Database: dbCfg.DatabaseName,
	}

	c, err := pq.NewConnectorConfig(cfg)
	db := sql.OpenDB(c)

	err = db.Ping()
	if err != nil {
		log.Fatal("Error while ping db")
	}

	sqlBytes, err := os.ReadFile(dbCfg.InitSqlFilepath)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(string(sqlBytes))
	if err != nil {
		log.Fatal(err)
	}

	return db
}
