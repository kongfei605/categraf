//go:build !no_logs

package config

import (
	"github.com/IBM/sarama"

	logsconfig "flashcat.cloud/categraf/config/logs"
	"flashcat.cloud/categraf/pkg/tls"
)

const (
	Docker     = "docker"
	Kubernetes = "kubernetes"
)

type (
	Logs struct {
		APIKey                string                       `json:"api_key" toml:"api_key"`
		Enable                bool                         `json:"enable" toml:"enable"`
		SendTo                string                       `json:"send_to" toml:"send_to"`
		SendType              string                       `json:"send_type" toml:"send_type"`
		UseCompression        bool                         `json:"use_compression" toml:"use_compression"`
		CompressionLevel      int                          `json:"compression_level" toml:"compression_level"`
		SendWithTLS           bool                         `json:"send_with_tls" toml:"send_with_tls"`
		BatchWait             int                          `json:"batch_wait" toml:"batch_wait"`
		RunPath               string                       `json:"run_path" toml:"run_path"`
		OpenFilesLimit        int                          `json:"open_files_limit" toml:"open_files_limit"`
		MaxTraverseLimit      int                          `json:"max_traverse_limit" toml:"max_traverse_limit"`
		MaxDepthLimit         int                          `json:"max_depth_limit" toml:"max_depth_limit"`
		ScanPeriod            int                          `json:"scan_period" toml:"scan_period"`
		FrameSize             int                          `json:"frame_size" toml:"frame_size"`
		CollectContainerAll   bool                         `json:"collect_container_all" toml:"collect_container_all"`
		ContainerInclude      []string                     `json:"container_include" toml:"container_include"`
		ContainerExclude      []string                     `json:"container_exclude" toml:"container_exclude"`
		GlobalProcessingRules []*logsconfig.ProcessingRule `json:"processing_rules" toml:"processing_rules"`
		Items                 []*logsconfig.LogsConfig     `json:"items" toml:"items"`
		Accuracy              string                       `toml:"accuracy" json:"accuracy"`
		KafkaConfig
		KubeConfig

		ChanSize            int `toml:"chan_size" json:"chan_size"`
		Pipeline            int `toml:"pipeline" json:"pipeline"`
		BatchMaxSize        int `toml:"batch_max_size" json:"batch_max_size"`
		BatchMaxContentSize int `toml:"batch_max_content_size" json:"batch_max_content_size"`
		BatchConcurrence    int `toml:"batch_max_concurrence" json:"batch_max_concurrence"`
		ProducerTimeout     int `toml:"producer_timeout" json:"producer_timeout"`

		EnableCollectContainer bool `json:"enable_collect_container" toml:"enable_collect_container"`
	}
	KafkaConfig struct {
		Topic   string   `json:"topic" toml:"topic"`
		Brokers []string `json:"brokers" toml:"brokers"`
		*sarama.Config

		CompressionCodec string `json:"compression_codec" toml:"compression_codec"`

		KafkaVersion     string `toml:"kafka_version"`
		SaslEnable       bool   `toml:"sasl_enable"`
		SaslMechanism    string `toml:"sasl_mechanism"`
		SaslVersion      int16  `toml:"sasl_version"`
		SaslHandshake    bool   `toml:"sasl_handshake"`
		SaslUser         string `toml:"sasl_user"`
		SaslPassword     string `toml:"sasl_password"`
		SaslAuthIdentity string `toml:"sasl_auth_identity"`

		CertificateAuth []string `toml:"certificate_authorities"`
		tls.ClientConfig
		PartitionStrategy string `toml:"partition_strategy"`
	}
	KubeConfig struct {
		KubeletHTTPPort  int    `json:"kubernetes_http_kubelet_port" toml:"kubernetes_http_kubelet_port"`
		KubeletHTTPSPort int    `json:"kubernetes_https_kubelet_port" toml:"kubernetes_https_kubelet_port"`
		KubeletTokenPath string `json:"kubelet_auth_token_path" toml:"kubelet_auth_token_path"`
		KubeletCAPath    string `json:"kubelet_client_ca" toml:"kubelet_client_ca"`
	}
)

func GetLogRunPath() string {
	if len(Config.Logs.RunPath) == 0 {
		Config.Logs.RunPath = "/opt/categraf/run"
	}
	return Config.Logs.RunPath
}

func LogRunPath(cfg *ConfigType) string {
	if cfg != nil && cfg.Logs.RunPath != "" {
		return cfg.Logs.RunPath
	}
	return "/opt/categraf/run"
}

func GetLogReadTimeout() int {
	return 30
}

func OpenLogsLimit() int {
	if Config.Logs.OpenFilesLimit == 0 {
		Config.Logs.OpenFilesLimit = 100
	}
	return Config.Logs.OpenFilesLimit
}

func OpenLogsLimitFor(cfg *ConfigType) int {
	if cfg != nil && cfg.Logs.OpenFilesLimit != 0 {
		return cfg.Logs.OpenFilesLimit
	}
	return 100
}

func MaxTraverseLimit() int {
	if Config.Logs.MaxTraverseLimit <= 0 {
		return 100000
	}
	return Config.Logs.MaxTraverseLimit
}

func MaxTraverseLimitFor(cfg *ConfigType) int {
	if cfg != nil && cfg.Logs.MaxTraverseLimit > 0 {
		return cfg.Logs.MaxTraverseLimit
	}
	return 100000
}

func MaxDepthLimit() int {
	if Config.Logs.MaxDepthLimit <= 0 {
		return 15
	}
	return Config.Logs.MaxDepthLimit
}

func MaxDepthLimitFor(cfg *ConfigType) int {
	if cfg != nil && cfg.Logs.MaxDepthLimit > 0 {
		return cfg.Logs.MaxDepthLimit
	}
	return 15
}

func FileScanPeriod() int {
	if Config.Logs.ScanPeriod == 0 {
		Config.Logs.ScanPeriod = 10
	}
	return Config.Logs.ScanPeriod
}

func FileScanPeriodFor(cfg *ConfigType) int {
	if cfg != nil && cfg.Logs.ScanPeriod != 0 {
		return cfg.Logs.ScanPeriod
	}
	return 10
}

func LogFrameSize() int {
	if Config.Logs.FrameSize == 0 {
		Config.Logs.FrameSize = 9000
	}
	return Config.Logs.FrameSize
}

func LogFrameSizeFor(cfg *ConfigType) int {
	if cfg != nil && cfg.Logs.FrameSize != 0 {
		return cfg.Logs.FrameSize
	}
	return 9000
}

func NumberOfPipelines() int {
	if Config.Logs.Pipeline == 0 {
		Config.Logs.Pipeline = 4
	}
	return Config.Logs.Pipeline
}

func NumberOfPipelinesFor(cfg *ConfigType) int {
	if cfg != nil && cfg.Logs.Pipeline != 0 {
		return cfg.Logs.Pipeline
	}
	return 4
}

func ChanSize() int {
	if Config.Logs.ChanSize == 0 {
		Config.Logs.ChanSize = 100
	}
	return Config.Logs.ChanSize
}

func ChanSizeFor(cfg *ConfigType) int {
	if cfg != nil && cfg.Logs.ChanSize != 0 {
		return cfg.Logs.ChanSize
	}
	return 100
}

func BatchMaxSize() int {
	if Config.Logs.BatchMaxSize == 0 {
		Config.Logs.BatchMaxSize = 100
	}
	if Config.Logs.BatchMaxSize < Config.Logs.ChanSize {
		Config.Logs.BatchMaxSize = Config.Logs.ChanSize
	}
	return Config.Logs.BatchMaxSize
}

func BatchMaxSizeFor(cfg *ConfigType) int {
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

func BatchMaxContentSize() int {
	if Config.Logs.BatchMaxContentSize == 0 {
		Config.Logs.BatchMaxContentSize = 1000000
	}
	return Config.Logs.BatchMaxContentSize
}

func BatchMaxContentSizeFor(cfg *ConfigType) int {
	if cfg != nil && cfg.Logs.BatchMaxContentSize != 0 {
		return cfg.Logs.BatchMaxContentSize
	}
	return 1000000
}

func BatchConcurrence() int {
	return Config.Logs.BatchConcurrence
}

func BatchConcurrenceFor(cfg *ConfigType) int {
	if cfg == nil {
		return 0
	}
	return cfg.Logs.BatchConcurrence
}

func ClientTimeout() int {
	if Config.Logs.ProducerTimeout == 0 {
		Config.Logs.ProducerTimeout = 10
	}
	return Config.Logs.ProducerTimeout
}

func ClientTimeoutFor(cfg *ConfigType) int {
	if cfg != nil && cfg.Logs.ProducerTimeout != 0 {
		return cfg.Logs.ProducerTimeout
	}
	return 10
}

func ValidatePodContainerID() bool {
	return false
}

func IsFeaturePresent(t string) bool {
	return false
}

func EnableCollectContainer() bool {
	if Version < "v0.3.58" {
		return Config.Logs.CollectContainerAll
	}
	return Config.Logs.EnableCollectContainer
}

func EnableCollectContainerFor(cfg *ConfigType) bool {
	if cfg == nil {
		return false
	}
	if Version < "v0.3.58" {
		return cfg.Logs.CollectContainerAll
	}
	return cfg.Logs.EnableCollectContainer
}

func GetContainerCollectAll() bool {
	return Config.Logs.CollectContainerAll
}

func GetContainerIncludeList() []string {
	if Config.Logs.ContainerInclude == nil {
		return []string{}
	}
	return Config.Logs.ContainerInclude
}

func GetContainerExcludeList() []string {
	if Config.Logs.ContainerExclude == nil {
		return []string{}
	}
	return Config.Logs.ContainerExclude
}
