//go:build !no_logs

package agent

import (
	"testing"

	"github.com/IBM/sarama"

	"flashcat.cloud/categraf/config"
)

func TestLogsConfigEqualIgnoresKafkaRuntimeConfig(t *testing.T) {
	oldCfg := &config.ConfigType{}
	newCfg := &config.ConfigType{}

	oldCfg.Logs.Enable = true
	newCfg.Logs.Enable = true
	oldCfg.Logs.SendType = "kafka"
	newCfg.Logs.SendType = "kafka"
	oldCfg.Logs.SendTo = "127.0.0.1:9092"
	newCfg.Logs.SendTo = "127.0.0.1:9092"
	oldCfg.Logs.Topic = "logs"
	newCfg.Logs.Topic = "logs"

	oldCfg.Logs.Config = sarama.NewConfig()
	oldCfg.Logs.Producer.Return.Successes = true
	oldCfg.Logs.Producer.Return.Errors = true
	oldCfg.Logs.ChannelBufferSize = 256
	oldCfg.Logs.Net.MaxOpenRequests = 2
	oldCfg.Logs.ProducerTimeout = 10

	if !logsConfigEqual(oldCfg, newCfg) {
		t.Fatal("expected kafka runtime config to be ignored")
	}
}

func TestLogsConfigEqualIgnoresKafkaTLSRuntimeUseTLS(t *testing.T) {
	oldCfg := &config.ConfigType{}
	newCfg := &config.ConfigType{}

	oldCfg.Logs.Enable = true
	newCfg.Logs.Enable = true
	oldCfg.Logs.SendType = "kafka"
	newCfg.Logs.SendType = "kafka"
	oldCfg.Logs.SendWithTLS = true
	newCfg.Logs.SendWithTLS = true
	oldCfg.Logs.UseTLS = true

	if !logsConfigEqual(oldCfg, newCfg) {
		t.Fatal("expected kafka send_with_tls runtime UseTLS to be ignored")
	}
}

func TestLogsConfigEqualDetectsExplicitProducerTimeoutChange(t *testing.T) {
	oldCfg := &config.ConfigType{}
	newCfg := &config.ConfigType{}

	oldCfg.Logs.Enable = true
	newCfg.Logs.Enable = true
	oldCfg.Logs.SendType = "kafka"
	newCfg.Logs.SendType = "kafka"
	oldCfg.Logs.ProducerTimeout = 10
	newCfg.Logs.ProducerTimeout = 20

	if logsConfigEqual(oldCfg, newCfg) {
		t.Fatal("expected explicit producer timeout change to require reload")
	}
}
