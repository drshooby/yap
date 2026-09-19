// Command yap runs a gossip-network simulation of in-process LLM agents.
package main

import (
	"fmt"
	"os"

	"github.com/drshooby/yap/internal/config"
	"github.com/drshooby/yap/internal/model"
	"github.com/drshooby/yap/internal/model/anthropic"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "yap:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Claude Note:
	// One client per provider rather than per cohort, so cohorts sharing a
	// provider share its connection pool and rate limiting. A cohort names its
	// provider explicitly; #7 validates that the name is in this map before the
	// first round rather than discovering it mid-run.
	clients := map[string]model.Client{
		"anthropic": model.WithRetry(anthropic.New(cfg.AnthropicAPIKey), 3),
	}
	_ = clients

	fmt.Println("usage: yap --config <experiment.yaml>")
	return nil
}
