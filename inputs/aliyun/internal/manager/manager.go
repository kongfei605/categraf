package manager

import (
	cms20190101 "github.com/alibabacloud-go/cms-20190101/v8/client"
	"unicode"

	cms2021101 "github.com/alibabacloud-go/cms-export-20211101/v2/client"
)

type (
	CmsV2Client struct {
		key, secret, region, endpoint string

		cmsv2 *cms2021101.Client
	}

	Manager struct {
		endpoint  string
		region    string
		apikey    string
		apiSecret string

		cms   *cms20190101.Client
		cmsv2 *cms2021101.Client

		metaClient map[string]interface{}
		// meta info agent
		sas   *SasClient
		ecs   *EcsClient
		polar *PolarDBClient
		kv    *KVClient
		rds   *RDSClient
	}
)

type Option func(*Manager) error

func New(key, secret, endpoint, region string, opts ...Option) (*Manager, error) {
	var (
		err error
	)

	m := &Manager{
		region:    region,
		endpoint:  endpoint,
		apikey:    key,
		apiSecret: secret,
	}
	for _, opt := range opts {
		err = opt(m)
		if err != nil {
			return nil, err
		}
	}
	return m, nil
}

func snakeCase(in string) string {
	runes := []rune(in)
	length := len(runes)

	var out []rune
	for i := 0; i < length; i++ {
		if i > 0 && unicode.IsUpper(runes[i]) && ((i+1 < length && unicode.IsLower(runes[i+1])) || unicode.IsLower(runes[i-1])) {
			out = append(out, '_')
		}
		out = append(out, unicode.ToLower(runes[i]))
	}

	return string(out)
}
