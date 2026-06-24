//go:build no_logs

package agent

import "flashcat.cloud/categraf/config"

func logsConfigEqual(oldCfg, newCfg *config.ConfigType) bool {
	return true
}
