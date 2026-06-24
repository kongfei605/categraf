//go:build no_logs

package agent

import "flashcat.cloud/categraf/config"

func logsConfigured(cfg *config.ConfigType) bool {
	return false
}
