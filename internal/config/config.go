package config

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	AnthropicAPIKey string `envconfig:"ANTHROPIC_API_KEY" required:"false"`
}

func Load() (*Config, error) {
	var c Config
	err := envconfig.Process("", &c)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return &c, nil
}
