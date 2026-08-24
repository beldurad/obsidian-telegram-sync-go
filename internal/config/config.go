package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	WebhookURL      string `yaml:"webhook_url"`
	WebhookEndpoint string `yaml:"webhook_endpoint"`
	Token           string `yaml:"token"`
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

func Load() (Config, error) {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		return Config{}, fmt.Errorf("CONFIG_PATH is not set")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return Config{}, fmt.Errorf("config file does not exist: %s", configPath)
	}

	var cfg Config

	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		return Config{}, fmt.Errorf("cannot read config: %s", configPath)
	}

	if err := loadSecrets(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func loadSecrets(cfg *Config) error {
	dir := os.Getenv("SECRETS_DIR")
	if dir == "" {
		dir = cfg.SecretsDir
	}
	if dir == "" {
		return fmt.Errorf("SECRETS_DIR is not set")
	}
	readSecret := func(name string) (string, error) {
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "", fmt.Errorf("cannot read secret %s: %v", name, err)
		}
		return strings.TrimSpace(string(content)), nil
	}
	dbPass, err := readSecret("db_pass")
	if err != nil {
		return err
	}
	tgToken, err := readSecret("tg_token")
	if err != nil {
		return err
	}
	githubClientID, err := readSecret("github_id")
	if err != nil {
		return err
	}
	githubClientSecret, err := readSecret("github_secret")
	if err != nil {
		return err
	}
	cfg.DatabaseConfig.Password = dbPass
	cfg.TelegramConfig.Token = tgToken
	cfg.GithubConfig.ClientID = githubClientID
	cfg.GithubConfig.ClientSecret = githubClientSecret
	return nil
}
