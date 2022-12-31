package moosefs

import (
	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"os"
	"time"
)

const inputName = `moosefs`
const description = `Retrieves moosefs metrics from mfscli`

// Mfs holds the configuration for the plugin.
type (
	Mfs struct {
		config.PluginConfig
		Instances []*Instance `toml:"instances"`
	}

	Instance struct {
		config.InstanceConfig

		Master string `toml:"master"`
		Port   int    `toml:"port"`
		// mfscli路径
		Client string `toml:"client"`

		// 默认UTC时间
		OverrideTimeZone string `toml:"override_timezone"`
	}

	Metric struct {
		MasterInfo   *prometheus.Desc
		MetaServer   *prometheus.Desc
		ChunkMetrix  *prometheus.Desc
		ChunkServer  *prometheus.Desc
		StorageClass *prometheus.Desc
	}
)

func (ins *Instance) Init() error {
	if len(ins.Master) == 0 {
		return types.ErrInstancesEmpty
	}
	if ins.Port == 0 {
		ins.Port = 9421
	}
	if len(ins.Client) == 0 {
		ins.Client = "mfscli"
	}

	_, err := time.LoadLocation(ins.OverrideTimeZone)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't parse timezone %q: %s", ins.OverrideTimeZone, err)
		return err
	}

	return nil
}
func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Mfs{}
	})
}

func (s *Mfs) GetInstances() []inputs.Instance {
	ret := make([]inputs.Instance, len(s.Instances))
	for i := 0; i < len(s.Instances); i++ {
		ret[i] = s.Instances[i]
	}
	return ret
}

// Description returns a one-sentence description on the input.
func (s *Mfs) Description() string {
	return description
}

func (ins *Instance) Gather(slist *types.SampleList) {
}
