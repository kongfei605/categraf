//go:build !no_logs

package agent

import coreconfig "flashcat.cloud/categraf/config"

func logsRunPath(cfg *coreconfig.ConfigType) string {
	if cfg != nil && cfg.Logs.RunPath != "" {
		return cfg.Logs.RunPath
	}
	return "/opt/categraf/run"
}

func logsOpenFilesLimit(cfg *coreconfig.ConfigType) int {
	if cfg != nil && cfg.Logs.OpenFilesLimit != 0 {
		return cfg.Logs.OpenFilesLimit
	}
	return 100
}

func logsMaxTraverseLimit(cfg *coreconfig.ConfigType) int {
	if cfg != nil && cfg.Logs.MaxTraverseLimit > 0 {
		return cfg.Logs.MaxTraverseLimit
	}
	return 100000
}

func logsMaxDepthLimit(cfg *coreconfig.ConfigType) int {
	if cfg != nil && cfg.Logs.MaxDepthLimit > 0 {
		return cfg.Logs.MaxDepthLimit
	}
	return 15
}

func logsFileScanPeriod(cfg *coreconfig.ConfigType) int {
	if cfg != nil && cfg.Logs.ScanPeriod != 0 {
		return cfg.Logs.ScanPeriod
	}
	return 10
}

func logsFrameSize(cfg *coreconfig.ConfigType) int {
	if cfg != nil && cfg.Logs.FrameSize != 0 {
		return cfg.Logs.FrameSize
	}
	return 9000
}

func logsPipelineCount(cfg *coreconfig.ConfigType) int {
	if cfg != nil && cfg.Logs.Pipeline != 0 {
		return cfg.Logs.Pipeline
	}
	return 4
}

func logsBatchMaxSize(cfg *coreconfig.ConfigType) int {
	batchMaxSize := 100
	chanSize := 100
	if cfg != nil {
		if cfg.Logs.BatchMaxSize != 0 {
			batchMaxSize = cfg.Logs.BatchMaxSize
		}
		if cfg.Logs.ChanSize != 0 {
			chanSize = cfg.Logs.ChanSize
		}
	}
	if batchMaxSize < chanSize {
		return chanSize
	}
	return batchMaxSize
}

func logsBatchMaxContentSize(cfg *coreconfig.ConfigType) int {
	if cfg != nil && cfg.Logs.BatchMaxContentSize != 0 {
		return cfg.Logs.BatchMaxContentSize
	}
	return 1000000
}

func logsBatchConcurrence(cfg *coreconfig.ConfigType) int {
	if cfg == nil {
		return 0
	}
	return cfg.Logs.BatchConcurrence
}

func logsEnableCollectContainer(cfg *coreconfig.ConfigType) bool {
	if cfg == nil {
		return false
	}
	if coreconfig.Version < "v0.3.58" {
		return cfg.Logs.CollectContainerAll
	}
	return cfg.Logs.EnableCollectContainer
}
