package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/yaml"
)

// Claude Note:
// RunConfig is the spec half of the Swarm CRD in docs/roadmap.md, parsed from a
// file in Phase 1 and from a Kubernetes object in Phase 2. The json tags are
// what make that work: sigs.k8s.io/yaml converts YAML through JSON, so these
// same tags serve both without a second set of yaml tags to keep in sync.
type RunConfig struct {
	Rounds        int      `json:"rounds"`
	TokenBudget   int      `json:"tokenBudget"`
	Seed          int64    `json:"seed"`
	PeersPerRound int      `json:"peersPerRound"`
	Cohorts       []Cohort `json:"cohorts"`
}

// Claude Note:
// A cohort is a group of agents sharing a model and persona. Provider is
// explicit rather than inferred from the model name: OpenAI's Go SDK uses typed
// constants, so there is no string to prefix-match on.
type Cohort struct {
	Name       string `json:"name"`
	Count      int    `json:"count"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Persona    string `json:"persona"`
	SeedBelief string `json:"seedBelief,omitempty"`
}

// Population is the total number of agents, which is the sum of cohort counts.
// There is no separate agents field to disagree with the cohort list.
func (c *RunConfig) Population() int {
	total := 0
	for _, cohort := range c.Cohorts {
		total += cohort.Count
	}
	return total
}

// LoadRun reads and validates a run config. A config that fails validation is
// returned along with its errors, so a caller can report what was parsed.
func LoadRun(path string) (*RunConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read run config: %w", err)
	}

	var rc RunConfig
	// Claude Note:
	// UnmarshalStrict rejects unknown fields. A typo like "peersPerRounds"
	// would otherwise parse as zero and fail validation with a confusing
	// message about the field the author thought they had set.
	if err := yaml.UnmarshalStrict(data, &rc); err != nil {
		return nil, fmt.Errorf("parse run config %s: %w", path, err)
	}

	if err := rc.Validate(); err != nil {
		return &rc, fmt.Errorf("invalid run config %s: %w", path, err)
	}
	return &rc, nil
}

// Validate reports every problem with a config rather than stopping at the
// first, so one pass fixes a broken file instead of one problem per run.
func (c *RunConfig) Validate() error {
	var problems []error

	if c.Rounds <= 0 {
		problems = append(problems, fmt.Errorf("rounds must be positive, got %d", c.Rounds))
	}
	if c.TokenBudget <= 0 {
		problems = append(problems, fmt.Errorf("tokenBudget must be positive, got %d", c.TokenBudget))
	}
	if c.PeersPerRound <= 0 {
		problems = append(problems, fmt.Errorf("peersPerRound must be positive, got %d", c.PeersPerRound))
	}
	if len(c.Cohorts) == 0 {
		problems = append(problems, errors.New("at least one cohort is required"))
	}

	seen := make(map[string]bool, len(c.Cohorts))
	for i, cohort := range c.Cohorts {
		// Claude Note:
		// Cohorts are identified by name in events and in per-cohort analysis,
		// so an unnamed or duplicated one makes a run's output ambiguous.
		switch {
		case cohort.Name == "":
			problems = append(problems, fmt.Errorf("cohort %d: name is required", i))
		case seen[cohort.Name]:
			problems = append(problems, fmt.Errorf("cohort %d: duplicate name %q", i, cohort.Name))
		default:
			seen[cohort.Name] = true
		}

		label := cohort.Name
		if label == "" {
			label = fmt.Sprintf("cohort %d", i)
		}

		if cohort.Count <= 0 {
			problems = append(problems, fmt.Errorf("%s: count must be positive, got %d", label, cohort.Count))
		}
		if cohort.Provider == "" {
			problems = append(problems, fmt.Errorf("%s: provider is required", label))
		}
		if cohort.Model == "" {
			problems = append(problems, fmt.Errorf("%s: model is required", label))
		}
		if cohort.Persona == "" {
			problems = append(problems, fmt.Errorf("%s: persona is required", label))
		}
	}

	// Claude Note:
	// An agent cannot talk to itself, so k peers requires k+1 agents. Checked
	// only once the population is known to be meaningful, since a population of
	// zero would otherwise report two problems for one cause.
	if pop := c.Population(); pop > 0 && c.PeersPerRound >= pop {
		problems = append(problems,
			fmt.Errorf("peersPerRound (%d) must be less than the population (%d)", c.PeersPerRound, pop))
	}

	return errors.Join(problems...)
}

// ValidateProviders reports cohorts naming a provider the run has no client
// for. Kept separate from Validate because it depends on what was wired up at
// startup rather than on the file alone.
func (c *RunConfig) ValidateProviders(available map[string]bool) error {
	var problems []error
	for _, cohort := range c.Cohorts {
		if !available[cohort.Provider] {
			problems = append(problems, fmt.Errorf(
				"cohort %q: unknown provider %q (have: %s)",
				cohort.Name, cohort.Provider, strings.Join(sortedKeys(available), ", ")))
		}
	}
	return errors.Join(problems...)
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Small and fixed, so an insertion sort keeps the output stable without
	// pulling in sort for one call site.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
