package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/beldurad/obsidian-telegram-sync-go/internal/config"
	"github.com/tanimutomo/sqlfile"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func formatToDSN(cfg config.DatabaseConfig) string {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s",
		cfg.Host,
		cfg.Port,
		cfg.User,
		cfg.Password,
		cfg.DatabaseName,
	)
	return dsn
}

func New(cfg config.DatabaseConfig) (*sql.DB, error) {
	const op = "sqlite.New"

	gormDB, err := gorm.Open(sqlite.Open(formatToDSN(cfg)), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	db, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	sf := sqlfile.New()

	if err := sf.File(cfg.InitSqlFilepath); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if _, err := sf.Exec(db); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return db, nil
}
