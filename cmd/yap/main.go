// Command yap runs a gossip-network simulation of in-process LLM agents.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/drshooby/yap/internal/config"
	"github.com/drshooby/yap/internal/model"
	"github.com/drshooby/yap/internal/model/anthropic"
)

// Claude Note:
// maxRetries is how many attempts a transient failure gets before the call is
// reported as failed. Three covers a rate limit spike without letting one agent
// stall a round for long.
const maxRetries = 3

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "yap:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "path to a run config YAML file")
	dryRun := flag.Bool("dry-run", false, "estimate the cost of the run and exit without calling any API")
	flag.Parse()

	if *configPath == "" {
		flag.Usage()
		return fmt.Errorf("--config is required")
	}

	runCfg, err := config.LoadRun(*configPath)
	if err != nil {
		return err
	}

	// Claude Note:
	// The estimate needs no credentials and makes no calls, so it runs before
	// anything that could fail on a missing key. Knowing a run will not fit its
	// budget is useful even without an API key configured.
	estimate := runCfg.Estimate()
	estimate.Write(os.Stdout)

	if *dryRun {
		return nil
	}

	env, err := config.Load()
	if err != nil {
		return err
	}

	clients, err := buildClients(env, runCfg)
	if err != nil {
		return err
	}

	fmt.Printf("\n%d agents over %d rounds, %d providers wired\n",
		runCfg.Population(), runCfg.Rounds, len(clients))
	return fmt.Errorf("the round loop is not implemented yet (#12)")
}

// Claude Note:
// One client per provider rather than per cohort, so cohorts sharing a provider
// share its connection pool and rate limiting. Only providers a cohort actually
// names are built, so a run using one provider does not require the other's key.
func buildClients(env *config.Config, runCfg *config.RunConfig) (map[string]model.Client, error) {
	needed := map[string]bool{}
	for _, cohort := range runCfg.Cohorts {
		needed[cohort.Provider] = true
	}

	known := map[string]func() (model.Client, error){
		"anthropic": func() (model.Client, error) {
			if env.AnthropicAPIKey == "" {
				return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set")
			}
			return model.WithRetry(anthropic.New(env.AnthropicAPIKey), maxRetries), nil
		},
	}

	available := map[string]bool{}
	for name := range known {
		available[name] = true
	}
	if err := runCfg.ValidateProviders(available); err != nil {
		return nil, err
	}

	clients := map[string]model.Client{}
	for name := range needed {
		client, err := known[name]()
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", name, err)
		}
		clients[name] = client
	}
	return clients, nil
}
