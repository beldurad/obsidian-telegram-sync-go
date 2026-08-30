package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	DBConfig     `yaml:"db"`
	TGConfig     `yaml:"telegram"`
	GithubConfig `yaml:"github"`
	ServerConfig `yaml:"server"`
	TLSConfig    `yaml:"tls"`
	SecretsDir   string `yaml:"secrets_dir"`
}

type DBConfig struct {
	Host         string `yaml:"host" env:"DB_HOST"`
	Port         uint16 `yaml:"port" env:"DB_PORT"`
	User         string `yaml:"user" env:"DB_USER"`
	Password     string `yaml:"password"`
	DatabaseName string `yaml:"name" env:"DB_NAME"`
}

func (c DBConfig) URL() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		url.QueryEscape(c.User),
		url.QueryEscape(c.Password),
		c.Host,
		c.Port,
		url.QueryEscape(c.DatabaseName),
	)
}

func LoadDB() (DBConfig, error) {
	const DB_PASS_FILE = "DB_PASS_FILE"
	var cfg DBConfig
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return DBConfig{}, err
	}
	passFile, ok := os.LookupEnv(DB_PASS_FILE)
	if !ok || passFile == "" {
		return DBConfig{}, fmt.Errorf("%s not set", DB_PASS_FILE)
	}
	pass, err := readSecret(passFile)
	if err != nil {
		return DBConfig{}, err
	}
	cfg.Password = pass
	if err := validateDBConfig(cfg); err != nil {
		return DBConfig{}, err
	}
	return cfg, nil
}
func validateDBConfig(cfg DBConfig) error {
	if cfg.Host == "" {
		return fmt.Errorf("Host must not be empty string")
	}
	if cfg.Port == 0 {
		return fmt.Errorf("Port must not be 0")
	}
	if cfg.User == "" {
		return fmt.Errorf("User must not be empty string")
	}
	return nil
}

type TGConfig struct {
	WebhookURL      string `yaml:"webhook_url" env:"TG_WEBHOOK_URL"`
	WebhookEndpoint string `yaml:"webhook_endpoint" env:"TG_WEBHOOK_ENDPOINT"`
	Token           string `yaml:"token"`
}

func LoadTG() (TGConfig, error) {
	const TG_TOKEN_FILE = "TG_TOKEN_FILE"
	var cfg TGConfig
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return TGConfig{}, err
	}
	tokenFile, ok := os.LookupEnv(TG_TOKEN_FILE)
	if !ok || tokenFile == "" {
		return TGConfig{}, fmt.Errorf("no %s env or empty", TG_TOKEN_FILE)
	}
	token, err := readSecret(tokenFile)
	if err != nil {
		return TGConfig{}, err
	}
	cfg.Token = token
	if err := validateTGConfig(cfg); err != nil {
		return TGConfig{}, err
	}
	return cfg, nil
}

func validateTGConfig(cfg TGConfig) error {
	if cfg.Token == "" {
		return fmt.Errorf("Token must not be empty string")
	}
	if cfg.WebhookURL == "" {
		return fmt.Errorf("Webhook url must not be empty string")
	}
	return nil
}

type GithubConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURL  string `yaml:"redirect_url" env:"GITHUB_REDIRECT"`
	Scopes       string `yaml:"scopes" env:"GITHUB_SCOPES"`
}

func LoadGithub() (GithubConfig, error) {
	const GITHUB_ID_FILE = "GITHUB_ID_FILE"
	const GITHUB_SECRET_FILE = "GITHUB_SECRET_FILE"

	var cfg GithubConfig
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return GithubConfig{}, err
	}
	idFile, ok := os.LookupEnv(GITHUB_ID_FILE)
	if !ok || idFile == "" {
		return GithubConfig{}, fmt.Errorf("no %s env or empty", GITHUB_ID_FILE)
	}
	id, err := readSecret(idFile)
	if err != nil {
		return GithubConfig{}, err
	}
	secretFile, ok := os.LookupEnv(GITHUB_SECRET_FILE)
	if !ok || secretFile == "" {
		return GithubConfig{}, fmt.Errorf("no %s env or empty", GITHUB_SECRET_FILE)
	}
	secret, err := readSecret(secretFile)
	if err != nil {
		return GithubConfig{}, err
	}
	cfg.ClientID = id
	cfg.ClientSecret = secret
	if err := validateGithubConfig(cfg); err != nil {
		return GithubConfig{}, err
	}
	return cfg, nil
}

func validateGithubConfig(cfg GithubConfig) error {
	if cfg.ClientID == "" {
		return fmt.Errorf("client id must not be empty string")
	}
	if cfg.ClientSecret == "" {
		return fmt.Errorf("client secret must not be empty")
	}
	if cfg.RedirectURL == "" {
		return fmt.Errorf("redirect url must not be empty string")
	}
	return nil
}

type ServerConfig struct {
	Addr string `yaml:"addr" env:"SERVER_ADDR"`
}

func LoadServer() (ServerConfig, error) {
	var cfg ServerConfig
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return ServerConfig{}, err
	}
	if err := validateServerConfig(cfg); err != nil {
		return ServerConfig{}, err
	}
	return cfg, nil
}

func validateServerConfig(cfg ServerConfig) error {
	if cfg.Addr == "" {
		return fmt.Errorf("server address must not be empty string")
	}
	return nil
}

type TLSConfig struct {
	CertFile string `yaml:"cert_file" env:"TLS_CERT"`
	KeyFile  string `yaml:"key_file" env:"TLS_KEY"`
}

func LoadTLS() (TLSConfig, error) {
	var cfg TLSConfig
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return TLSConfig{}, err
	}
	if err := validateTLSConfig(cfg); err != nil {
		return TLSConfig{}, err
	}
	return cfg, nil
}

func validateTLSConfig(cfg TLSConfig) error {
	if cfg.CertFile == "" {
		return fmt.Errorf("cert file must not be empty string")
	}
	if cfg.KeyFile == "" {
		return fmt.Errorf("key file must not be empty string")
	}
	return nil
}

// [LoadEnv] loads configuration from environment variables
func LoadEnv() (Config, error) {
	db, err := LoadDB()
	if err != nil {
		return Config{}, err
	}
	tg, err := LoadTG()
	if err != nil {
		return Config{}, err
	}
	github, err := LoadGithub()
	if err != nil {
		return Config{}, err
	}
	server, err := LoadServer()
	if err != nil {
		return Config{}, err
	}
	tls, err := LoadTLS()
	if err != nil {
		return Config{}, err
	}
	return Config{
		DBConfig:     db,
		TGConfig:     tg,
		GithubConfig: github,
		ServerConfig: server,
		TLSConfig:    tls,
	}, nil
}

func Load() (Config, error) {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		return Config{}, fmt.Errorf("CONFIG_PATH is not set")
	}

	var cfg Config

	if err := loadConfig(configPath, &cfg); err != nil {
		return Config{}, err
	}

	if err := loadSecrets(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func loadConfig(path string, dst any) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("config file: %w", err)
	}

	if err := cleanenv.ReadConfig(path, dst); err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	return nil
}

func readSecret(name string) (string, error) {
	const SECRETS_DIR = "SECRETS_DIR"
	dir := os.Getenv(SECRETS_DIR)
	if dir == "" {
		return "", fmt.Errorf("%s is not set", SECRETS_DIR)
	}
	content, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "", fmt.Errorf("cannot read secret %s: %v", name, err)
	}
	return strings.TrimSpace(string(content)), nil
}

func loadSecrets(cfg *Config) error {
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
	cfg.DBConfig.Password = dbPass
	cfg.TGConfig.Token = tgToken
	cfg.GithubConfig.ClientID = githubClientID
	cfg.GithubConfig.ClientSecret = githubClientSecret
	return nil
}
