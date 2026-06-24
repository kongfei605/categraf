package agent

import (
	"errors"
	"log"

	"flashcat.cloud/categraf/config"
)

type Agent struct {
	agents []AgentModule
}

const (
	MetricsAgentName    = "metrics-agent"
	LogsAgentName       = "logs-agent"
	PrometheusAgentName = "prometheus-agent"
	IbexAgentName       = "ibex-agent"
)

// AgentModule is the interface for agent modules
// Use NewXXXAgent() to create a new agent module
// if the agent module is not needed, return nil
type AgentModule interface {
	Start() error
	Stop() error
	Name() string
}

func NewAgent() (*Agent, error) {
	agent := &Agent{
		agents: []AgentModule{
			NewMetricsAgent(),
			NewLogsAgent(config.Config),
			NewPrometheusAgent(),
			NewIbexAgent(),
		},
	}
	for _, ag := range agent.agents {
		if ag != nil {
			return agent, nil
		}
	}
	return nil, errors.New("no valid running agents, please check configuration")
}

func (a *Agent) Start() {
	log.Println("I! agent starting")
	for _, agent := range a.agents {
		if agent == nil {
			continue
		}
		if err := agent.Start(); err != nil {
			log.Printf("E! start [%T] err: [%+v]", agent, err)
		} else {
			log.Printf("I! [%T] started", agent)
		}
	}
	log.Println("I! agent started")
}

func (a *Agent) Stop() {
	log.Println("I! agent stopping")
	for _, agent := range a.agents {
		if agent == nil {
			continue
		}
		if err := agent.Stop(); err != nil {
			log.Printf("E! stop [%T] err: [%+v]", agent, err)
		} else {
			log.Printf("I! [%T] stopped", agent)
		}
	}
	log.Println("I! agent stopped")
}

func (a *Agent) GetAgent(name string) AgentModule {
	for _, agent := range a.agents {
		if agent == nil || agent.Name() != name {
			continue
		}
		return agent
	}
	return nil
}

func (a *Agent) SetAgent(name string, newAgent AgentModule) {
	for i, agent := range a.agents {
		if agent == nil || agent.Name() != name {
			continue
		}
		a.agents[i] = newAgent
		return
	}
	if newAgent != nil {
		a.agents = append(a.agents, newAgent)
	}
}

func (a *Agent) StopAgent(name string) error {
	agent := a.GetAgent(name)
	if agent == nil {
		return nil
	}
	return agent.Stop()
}
