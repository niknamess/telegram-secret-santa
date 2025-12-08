package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Telegram struct {
		BotToken string
		Admins   []string
	}
	Redis struct {
		Host     string
		Port     string
		Password string
		DB       int
	}
	TriggerWords []string
}

func LoadFromEnv() (*Config, error) {
	cfg := &Config{}

	cfg.Telegram.BotToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	if cfg.Telegram.BotToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	adminsStr := os.Getenv("TELEGRAM_ADMINS")
	if adminsStr != "" {
		cfg.Telegram.Admins = strings.Split(adminsStr, ",")
		for i := range cfg.Telegram.Admins {
			cfg.Telegram.Admins[i] = strings.TrimSpace(cfg.Telegram.Admins[i])
		}
	}

	cfg.Redis.Host = os.Getenv("REDIS_HOST")
	if cfg.Redis.Host == "" {
		cfg.Redis.Host = "localhost"
	}

	cfg.Redis.Port = os.Getenv("REDIS_PORT")
	if cfg.Redis.Port == "" {
		cfg.Redis.Port = "6379"
	}

	cfg.Redis.Password = os.Getenv("REDIS_PASSWORD")

	dbStr := os.Getenv("REDIS_DB")
	if dbStr != "" {
		db, err := strconv.Atoi(dbStr)
		if err == nil {
			cfg.Redis.DB = db
		}
	}

	triggerWordsStr := os.Getenv("TRIGGER_WORDS")
	if triggerWordsStr != "" {
		cfg.TriggerWords = strings.Split(triggerWordsStr, ",")
		for i := range cfg.TriggerWords {
			cfg.TriggerWords[i] = strings.TrimSpace(cfg.TriggerWords[i])
		}
	}

	return cfg, nil
}
