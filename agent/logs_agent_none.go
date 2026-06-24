//go:build no_logs

package agent

import coreconfig "flashcat.cloud/categraf/config"

type LogsAgent struct {
}

func NewLogsAgent(cfg *coreconfig.ConfigType) AgentModule {
	return nil
}

func (la *LogsAgent) Start() error {
	return nil
}

func (la *LogsAgent) Stop() error {
	return nil
}

func (la *LogsAgent) Name() string {
	return LogsAgentName
}
