package postgres

import (
	"database/sql"
	"os"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/config"

	"github.com/lib/pq"
)

func New(dbCfg config.DatabaseConfig) (*sql.DB, error) {
	cfg := pq.Config{
		Host:     dbCfg.Host,
		Port:     dbCfg.Port,
		User:     dbCfg.User,
		Password: dbCfg.Password,
		Database: dbCfg.DatabaseName,
		SSLMode:  pq.SSLModeDisable,
	}

	c, err := pq.NewConnectorConfig(cfg)
	db := sql.OpenDB(c)

	err = db.Ping()
	if err != nil {
		return nil, err
	}

	sqlBytes, err := os.ReadFile(dbCfg.InitSqlFilepath)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(string(sqlBytes))
	if err != nil {
		return nil, err
	}

	return db, nil
}
