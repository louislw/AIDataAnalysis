package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the process environment described in DESIGN.md.
type Config struct {
	Addr                   string
	DataRoot               string
	JWTSecret              string
	LLMBaseURL             string
	LLMAPIKey              string
	LLMModel               string
	LLMTimeout             time.Duration
	AgentMaxLoop           int
	AgentMaxRows           int
	AgentHistoryTurns      int
	BootstrapAdminPassword string
	BootstrapUserPassword  string
}

func Load() (Config, error) {
	cfg := Config{
		Addr:                   getenv("APP_ADDR", ":8081"),
		DataRoot:               getenv("DATA_ROOT", "./data"),
		JWTSecret:              os.Getenv("JWT_SECRET"),
		LLMBaseURL:             strings.TrimRight(os.Getenv("LLM_BASE_URL"), "/"),
		LLMAPIKey:              os.Getenv("LLM_API_KEY"),
		LLMModel:               os.Getenv("LLM_MODEL"),
		LLMTimeout:             120 * time.Second,
		AgentMaxLoop:           6,
		AgentMaxRows:           50,
		AgentHistoryTurns:      6,
		BootstrapAdminPassword: os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
		BootstrapUserPassword:  os.Getenv("BOOTSTRAP_USER_PASSWORD"),
	}
	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET 未设置")
	}
	if v := os.Getenv("LLM_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("LLM_TIMEOUT: %w", err)
		}
		cfg.LLMTimeout = d
	}
	var err error
	if cfg.AgentMaxLoop, err = getenvInt("AGENT_MAX_LOOP", cfg.AgentMaxLoop); err != nil {
		return Config{}, err
	}
	if cfg.AgentMaxRows, err = getenvInt("AGENT_MAX_ROWS", cfg.AgentMaxRows); err != nil {
		return Config{}, err
	}
	if cfg.AgentHistoryTurns, err = getenvInt("AGENT_HISTORY_TURNS", cfg.AgentHistoryTurns); err != nil {
		return Config{}, err
	}
	if cfg.AgentMaxLoop < 1 || cfg.AgentMaxRows < 1 || cfg.AgentHistoryTurns < 1 {
		return Config{}, fmt.Errorf("AGENT_MAX_LOOP、AGENT_MAX_ROWS、AGENT_HISTORY_TURNS 必须大于 0")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}
