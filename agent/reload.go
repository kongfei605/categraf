package agent

import (
	"fmt"
	"reflect"

	"flashcat.cloud/categraf/config"
)

type ReloadPlan struct {
	NewLogsAgent    AgentModule
	LogsReloaded    bool
	RestartRequired []string
}

func (a *Agent) PrepareReload(newCfg *config.ConfigType) (*ReloadPlan, error) {
	if newCfg == nil {
		return nil, fmt.Errorf("new config is nil")
	}

	oldCfg := config.Config
	plan := &ReloadPlan{}

	if oldCfg == nil || !reflect.DeepEqual(oldCfg.Writers, newCfg.Writers) ||
		!reflect.DeepEqual(oldCfg.WriterOpt, newCfg.WriterOpt) {
		plan.RestartRequired = append(plan.RestartRequired, "writer")
	}
	if oldCfg == nil || !reflect.DeepEqual(oldCfg.Global, newCfg.Global) {
		plan.RestartRequired = append(plan.RestartRequired, "global")
	}
	if oldCfg == nil || !reflect.DeepEqual(oldCfg.HTTP, newCfg.HTTP) {
		plan.RestartRequired = append(plan.RestartRequired, "http")
	}
	if oldCfg == nil || !reflect.DeepEqual(oldCfg.Heartbeat, newCfg.Heartbeat) {
		plan.RestartRequired = append(plan.RestartRequired, "heartbeat")
	}
	if oldCfg == nil || !reflect.DeepEqual(oldCfg.Ibex, newCfg.Ibex) {
		plan.RestartRequired = append(plan.RestartRequired, "ibex")
	}
	if oldCfg == nil || !reflect.DeepEqual(oldCfg.Log, newCfg.Log) {
		plan.RestartRequired = append(plan.RestartRequired, "log")
	}
	if oldCfg == nil || !reflect.DeepEqual(oldCfg.Prometheus, newCfg.Prometheus) {
		plan.RestartRequired = append(plan.RestartRequired, "prometheus")
	}
	if oldCfg == nil || !reflect.DeepEqual(oldCfg.HTTPProviderConfig, newCfg.HTTPProviderConfig) {
		plan.RestartRequired = append(plan.RestartRequired, "http_provider")
	}

	plan.LogsReloaded = true
	plan.NewLogsAgent = NewLogsAgent(newCfg)
	if logsConfigured(newCfg) && plan.NewLogsAgent == nil {
		return nil, fmt.Errorf("failed to initialize logs agent from new config")
	}

	return plan, nil
}
