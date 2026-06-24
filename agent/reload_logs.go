//go:build !no_logs

package agent

import "flashcat.cloud/categraf/config"

func logsConfigured(cfg *config.ConfigType) bool {
	return cfg != nil &&
		cfg.Logs.Enable &&
		(len(cfg.Logs.Items) > 0 || cfg.Logs.EnableCollectContainer)
}
