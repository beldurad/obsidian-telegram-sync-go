package config

import (
	"fmt"

	"github.com/ilyakaznacheev/cleanenv"
)

type MigrateConfig struct {
	DBConfig
	MigrationsDir string `env:"MIGRATE_DIR"`
}

func LoadMigrate() (MigrateConfig, error) {
	db, err := LoadDB()
	if err != nil {
		return MigrateConfig{}, err
	}
	var cfg MigrateConfig
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return MigrateConfig{}, err
	}
	cfg.DBConfig = db
	if err := validateMigrationConfig(cfg); err != nil {
		return MigrateConfig{}, err
	}
	return cfg, nil
}

func validateMigrationConfig(cfg MigrateConfig) error {
	if err := validateDBConfig(cfg.DBConfig); err != nil {
		return err
	}
	if cfg.MigrationsDir == "" {
		return fmt.Errorf("MigrationsDir must not be empty string")
	}
	return nil
}
