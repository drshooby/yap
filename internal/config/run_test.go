package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig puts a config on disk for LoadRun to read.
func writeConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "run.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

const validConfig = `
rounds: 50
tokenBudget: 2000000
seed: 42
peersPerRound: 3
cohorts:
  - name: cultists
    count: 5
    provider: anthropic
    model: claude-sonnet-5
    persona: "Convinced."
    seedBelief: "The Vantril Principle."
  - name: crowd
    count: 195
    provider: anthropic
    model: claude-haiku-4-5
    persona: "Skeptical."
`

// The done-when depends on the YAML shape in docs/roadmap.md parsing into the
// fields the run actually reads.
func TestLoadRun(t *testing.T) {
	cfg, err := LoadRun(writeConfig(t, validConfig))
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}

	if cfg.Rounds != 50 {
		t.Errorf("Rounds = %d, want 50", cfg.Rounds)
	}
	if cfg.TokenBudget != 2_000_000 {
		t.Errorf("TokenBudget = %d, want 2000000", cfg.TokenBudget)
	}
	if cfg.Seed != 42 {
		t.Errorf("Seed = %d, want 42", cfg.Seed)
	}
	if cfg.PeersPerRound != 3 {
		t.Errorf("PeersPerRound = %d, want 3", cfg.PeersPerRound)
	}
	if len(cfg.Cohorts) != 2 {
		t.Fatalf("got %d cohorts, want 2", len(cfg.Cohorts))
	}

	first := cfg.Cohorts[0]
	if first.Name != "cultists" || first.Count != 5 {
		t.Errorf("cohort 0 = %+v, want cultists/5", first)
	}
	if first.Provider != "anthropic" || first.Model != "claude-sonnet-5" {
		t.Errorf("cohort 0 provider/model = %q/%q", first.Provider, first.Model)
	}
	if first.SeedBelief == "" {
		t.Error("cohort 0 lost its seedBelief")
	}
	if cfg.Cohorts[1].SeedBelief != "" {
		t.Error("cohort 1 gained a seedBelief it does not declare")
	}
}

// Population is the sum of cohort counts. There is no agents field, which is
// what removes the possibility of the two disagreeing.
func TestPopulation(t *testing.T) {
	cfg, err := LoadRun(writeConfig(t, validConfig))
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if got, want := cfg.Population(), 200; got != want {
		t.Errorf("Population() = %d, want %d", got, want)
	}
}

// A misspelled field must be rejected rather than silently parsing as zero and
// failing validation with a message about a field the author thought they set.
func TestUnknownFieldRejected(t *testing.T) {
	_, err := LoadRun(writeConfig(t, `
rounds: 50
tokenBudget: 2000000
peersPerRound: 3
peersPerRounds: 4
cohorts:
  - name: crowd
    count: 10
    provider: anthropic
    model: claude-haiku-4-5
    persona: "x"
`))
	if err == nil {
		t.Fatal("LoadRun accepted an unknown field")
	}
	if !strings.Contains(err.Error(), "peersPerRounds") {
		t.Errorf("error = %q, want it to name the unknown field", err)
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     RunConfig
		wantErr string
	}{
		{
			name:    "no cohorts",
			cfg:     RunConfig{Rounds: 1, TokenBudget: 1, PeersPerRound: 1},
			wantErr: "at least one cohort",
		},
		{
			name: "zero rounds",
			cfg: RunConfig{
				Rounds: 0, TokenBudget: 1, PeersPerRound: 1,
				Cohorts: []Cohort{{Name: "a", Count: 5, Provider: "anthropic", Model: "m", Persona: "p"}},
			},
			wantErr: "rounds must be positive",
		},
		{
			name: "negative budget",
			cfg: RunConfig{
				Rounds: 1, TokenBudget: -1, PeersPerRound: 1,
				Cohorts: []Cohort{{Name: "a", Count: 5, Provider: "anthropic", Model: "m", Persona: "p"}},
			},
			wantErr: "tokenBudget must be positive",
		},
		{
			name: "duplicate cohort names",
			cfg: RunConfig{
				Rounds: 1, TokenBudget: 1, PeersPerRound: 1,
				Cohorts: []Cohort{
					{Name: "same", Count: 5, Provider: "anthropic", Model: "m", Persona: "p"},
					{Name: "same", Count: 5, Provider: "anthropic", Model: "m", Persona: "p"},
				},
			},
			wantErr: `duplicate name "same"`,
		},
		{
			name: "missing provider",
			cfg: RunConfig{
				Rounds: 1, TokenBudget: 1, PeersPerRound: 1,
				Cohorts: []Cohort{{Name: "a", Count: 5, Model: "m", Persona: "p"}},
			},
			wantErr: "provider is required",
		},
		{
			name: "peers not less than population",
			cfg: RunConfig{
				Rounds: 1, TokenBudget: 1, PeersPerRound: 5,
				Cohorts: []Cohort{{Name: "a", Count: 5, Provider: "anthropic", Model: "m", Persona: "p"}},
			},
			wantErr: "must be less than the population",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil {
				t.Fatalf("Validate() accepted %+v", tt.cfg)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// Validation reports every problem at once. Fixing a broken config one error
// per run is the thing this avoids.
func TestValidationReportsEveryProblem(t *testing.T) {
	cfg := RunConfig{
		Rounds:        0,
		TokenBudget:   0,
		PeersPerRound: 0,
		Cohorts:       []Cohort{{Name: "", Count: 0}},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() accepted an empty config")
	}

	for _, want := range []string{
		"rounds must be positive",
		"tokenBudget must be positive",
		"peersPerRound must be positive",
		"name is required",
		"count must be positive",
		"provider is required",
		"model is required",
		"persona is required",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error is missing %q:\n%v", want, err)
		}
	}
}

// A valid config must produce no errors at all, or the validator is useless.
func TestValidConfigPasses(t *testing.T) {
	cfg, err := LoadRun(writeConfig(t, validConfig))
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() rejected a valid config: %v", err)
	}
}

// Providers are checked against what was actually wired up at startup, which
// the file alone cannot know.
func TestValidateProviders(t *testing.T) {
	cfg := RunConfig{Cohorts: []Cohort{
		{Name: "a", Provider: "anthropic"},
		{Name: "b", Provider: "openai"},
	}}

	if err := cfg.ValidateProviders(map[string]bool{"anthropic": true, "openai": true}); err != nil {
		t.Errorf("rejected providers that are available: %v", err)
	}

	err := cfg.ValidateProviders(map[string]bool{"anthropic": true})
	if err == nil {
		t.Fatal("accepted a cohort naming an unavailable provider")
	}
	if !strings.Contains(err.Error(), `cohort "b"`) || !strings.Contains(err.Error(), `"openai"`) {
		t.Errorf("error = %q, want it to name the cohort and the provider", err)
	}
	// The message should say what is available, or the fix is guesswork.
	if !strings.Contains(err.Error(), "anthropic") {
		t.Errorf("error = %q, want it to list the available providers", err)
	}
}

// The configs in experiments/ are shipped in the repo, so a change that breaks
// them should fail here rather than when someone runs one.
func TestShippedExperimentsAreValid(t *testing.T) {
	paths, err := filepath.Glob("../../experiments/*.yaml")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no configs found in experiments/")
	}

	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if _, err := LoadRun(path); err != nil {
				t.Errorf("LoadRun: %v", err)
			}
		})
	}
}
