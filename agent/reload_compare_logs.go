//go:build !no_logs

package agent

import (
	"reflect"

	"flashcat.cloud/categraf/config"
	logsconfig "flashcat.cloud/categraf/config/logs"
)

func logsConfigEqual(oldCfg, newCfg *config.ConfigType) bool {
	if oldCfg == nil || newCfg == nil {
		return oldCfg == newCfg
	}

	oldLogs := comparableLogs(oldCfg.Logs)
	newLogs := comparableLogs(newCfg.Logs)
	return reflect.DeepEqual(oldLogs, newLogs)
}

func comparableLogs(logs config.Logs) config.Logs {
	cfg := func() *config.ConfigType {
		return &config.ConfigType{Logs: logs}
	}
	logs.RunPath = config.LogRunPath(cfg())
	logs.OpenFilesLimit = config.OpenLogsLimitFor(cfg())
	logs.MaxTraverseLimit = config.MaxTraverseLimitFor(cfg())
	logs.MaxDepthLimit = config.MaxDepthLimitFor(cfg())
	logs.ScanPeriod = config.FileScanPeriodFor(cfg())
	logs.FrameSize = config.LogFrameSizeFor(cfg())
	logs.Pipeline = config.NumberOfPipelinesFor(cfg())
	logs.ChanSize = config.ChanSizeFor(cfg())
	logs.BatchMaxSize = config.BatchMaxSizeFor(&config.ConfigType{Logs: logs})
	logs.BatchMaxContentSize = config.BatchMaxContentSizeFor(cfg())
	logs.ProducerTimeout = config.ClientTimeoutFor(cfg())
	if logs.SendType == "kafka" && logs.SendWithTLS {
		logs.UseTLS = true
	}
	logs.Config = nil
	logs.GlobalProcessingRules = cloneComparableProcessingRules(logs.GlobalProcessingRules)
	logs.Items = cloneComparableLogsConfigs(logs.Items)
	return logs
}

func cloneComparableLogsConfigs(items []*logsconfig.LogsConfig) []*logsconfig.LogsConfig {
	if items == nil {
		return nil
	}
	cloned := make([]*logsconfig.LogsConfig, len(items))
	for i, item := range items {
		if item == nil {
			continue
		}
		copied := *item
		copied.Channel = nil
		copied.ProcessingRules = cloneComparableProcessingRules(item.ProcessingRules)
		cloned[i] = &copied
	}
	return cloned
}

func cloneComparableProcessingRules(rules []*logsconfig.ProcessingRule) []*logsconfig.ProcessingRule {
	if rules == nil {
		return nil
	}
	cloned := make([]*logsconfig.ProcessingRule, len(rules))
	for i, rule := range rules {
		if rule == nil {
			continue
		}
		copied := *rule
		copied.Regex = nil
		copied.Placeholder = nil
		cloned[i] = &copied
	}
	return cloned
}
