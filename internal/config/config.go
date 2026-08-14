package config

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	DatabaseConfig `yaml:"db"`
	TelegramConfig `yaml:"telegram"`
	GithubConfig   `yaml:"github"`
	ServerConfig   `yaml:"server"`
	SecretsDir     string `yaml:"secrets_dir"`
}

type DatabaseConfig struct {
	Host            string `yaml:"host"`
	Port            uint16 `yaml:"port"`
	User            string `yaml:"user"`
	Password        string `yaml:"password"`
	DatabaseName    string `yaml:"name"`
	InitSqlFilepath string `yaml:"init_sql_filepath"`
}

type TelegramConfig struct {
	Token string `yaml:"token"`
}

type GithubConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURL  string `yaml:"redirect_url"`
	Scopes       string `yaml:"scopes"`
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
}

func getenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%v not set", key)
		return ""
	}
	return v
}

func MustLoad() Config {
	dbHost := getenv("DB_HOST")
	dbPort, err := strconv.Atoi(getenv("DB_PORT"))
	if err != nil {
		log.Fatal("DB_PORT is not int")
	}
	dbUser := getenv("DB_USER")
	dbPass := getenv("DB_PASS")
	sqlFilepath := getenv("INIT_SQL_FILEPATH")
	dbCfg := DatabaseConfig{
		Host:            dbHost,
		Port:            uint16(dbPort),
		User:            dbUser,
		Password:        dbPass,
		InitSqlFilepath: sqlFilepath,
	}

	githubRedirect := getenv("GITHUB_REDIRECT")
	githubScopes := getenv("GITHUB_SCOPES")
	githubID := getenv("GITHUB_ID")
	githubSecret := getenv("GITHUB_SECRET")

	githubCfg := GithubConfig{
		RedirectURL:  githubRedirect,
		Scopes:       githubScopes,
		ClientID:     githubID,
		ClientSecret: githubSecret,
	}

	tgCfg := TelegramConfig{
		Token: getenv("TG_TOKEN"),
	}

	serverCfg := ServerConfig{
		Addr: getenv("HTTP_ADDR"),
	}

	return Config{
		DatabaseConfig: dbCfg,
		GithubConfig:   githubCfg,
		TelegramConfig: tgCfg,
		ServerConfig:   serverCfg,
	}
}
