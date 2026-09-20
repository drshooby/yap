package agent

import (
	"fmt"

	"github.com/drshooby/yap/internal/config"
	"github.com/drshooby/yap/internal/events"
)

type Agent struct {
	ID     string
	Cohort *config.Cohort
	Belief string
	Inbox  []events.Message
	Seen   map[events.MessageID]bool
}

const DefaultBeliefString = "no particular view yet"

func (a *Agent) Message(id, parent events.MessageID) events.Message {
	return events.Message{
		ID:           id,
		ParentID:     parent,
		Sender:       a.ID,
		SenderCohort: a.Cohort.Name,
		Text:         a.Belief,
	}
}

func (a *Agent) Receive(msg events.Message) bool {
	if a.Seen[msg.ID] {
		return false
	}
	a.Seen[msg.ID] = true
	a.Inbox = append(a.Inbox, msg)
	return true
}

func (a *Agent) Drain() []events.Message {
	messages := a.Inbox
	a.Inbox = nil
	return messages
}

func FlattenCohortsToAgents(cohorts []config.Cohort) []*Agent {
	var agents []*Agent
	id := 0
	for _, cohort := range cohorts {
		for range cohort.Count {
			a := Agent{
				ID:     fmt.Sprintf("agent-%03d", id),
				Cohort: &cohort,
				Belief: func() string {
					if cohort.SeedBelief == "" {
						return DefaultBeliefString
					}
					return cohort.SeedBelief
				}(),
				Seen: make(map[events.MessageID]bool),
			}
			agents = append(agents, &a)
			id++
		}
	}
	return agents
}
