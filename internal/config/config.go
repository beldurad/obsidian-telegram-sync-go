package config

import (
	"log"
	"os"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	DatabaseConfig `yaml:"db"`
	TelegramConfig `yaml:"telegram"`
	GithubConfig   `yaml:"github"`
	ServerConfig   `yaml:"server"`
	TLSConfig      `yaml:"tls"`
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

type TLSConfig struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
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
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		log.Fatal("CONFIG_PATH is not set")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("config file does not exist: %s", configPath)
	}

	var cfg Config

	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		log.Fatalf("cannot read config: %s", configPath)
	}

	return cfg
}
