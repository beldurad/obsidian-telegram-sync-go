package config

import "github.com/ilyakaznacheev/cleanenv"

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
	return cfg, nil
}
