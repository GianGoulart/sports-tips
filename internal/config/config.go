package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL  string
	JWTSecret    string
	OddsAPIKey   string
	OddsPapiKey  string
	ServerPort   string
}

func Load() (*Config, error) {
	_ = godotenv.Load() // no error if .env missing (prod uses real env)

	cfg := &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		OddsAPIKey:  os.Getenv("ODDS_API_KEY"),
		OddsPapiKey: os.Getenv("ODDSPAPI_KEY"),
		ServerPort:  os.Getenv("SERVER_PORT"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET required")
	}
	if cfg.ServerPort == "" {
		cfg.ServerPort = "8080"
	}

	return cfg, nil
}
