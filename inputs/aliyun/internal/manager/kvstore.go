package manager

import (
	"fmt"
	openapiv1 "github.com/alibabacloud-go/darabonba-openapi/client"
	kvstore "github.com/alibabacloud-go/r-kvstore-20150101/v2/client"
	"github.com/alibabacloud-go/tea/tea"
)

const (
	kvNamespace = "acs_kvstore"
)

type (
	KVClient struct {
		key, secret      string
		region, endpoint string
		*kvstore.Client
	}
)

func (m *Manager) DescribeKVInfo() ([]*kvstore.DescribeInstancesResponseBodyInstancesKVStoreInstance, error) {

	req := new(kvstore.DescribeInstancesRequest)
	req.SetRegionId(m.region)
	req.SetPageSize(100)
	req.SetPageNumber(DefaultPageNum)
	resp, err := m.kv.DescribeInstances(req)
	if err != nil {
		return nil, err
	}
	result := make([]*kvstore.DescribeInstancesResponseBodyInstancesKVStoreInstance, 0, 100)
	result = append(result, resp.Body.Instances.KVStoreInstance...)

	totalCount := resp.Body.TotalCount
	pageSize, pageNum := kvPageCaculator(int(*totalCount))
	req.SetPageSize(pageSize)
	var i int32
	for i = 2; i < 2+pageNum; i++ {
		req.SetPageNumber(i)
		resp, err := m.kv.DescribeInstances(req)
		if err != nil {
			return nil, err
		}
		result = append(result, resp.Body.Instances.KVStoreInstance...)
	}
	return resp.Body.Instances.KVStoreInstance, nil
}
func NewKVClient(key, secret, region string) Option {
	if len(key) == 0 {
		panic("accessKey for kvstore is required")
	}
	if len(secret) == 0 {
		panic("accessSecret for kvstore is required")
	}
	if len(region) == 0 {
		panic("region for kvstore is required")
	}

	return func(m *Manager) error {
		kvConfig := &openapiv1.Config{
			AccessKeyId:     tea.String(key),
			AccessKeySecret: tea.String(secret),
			RegionId:        tea.String(region),
		}

		kv, err := kvstore.NewClient(kvConfig)
		if err != nil {
			return err
		}
		m.kv = &KVClient{
			key:    key,
			secret: secret,
			region: region,
			Client: kv,
		}
		return nil
	}
}

func (m *Manager) KVKey(instanceID string) string {
	return fmt.Sprintf("%s||%s", kvNamespace, instanceID)
}

func kvPageCaculator(totalcount int) (int32, int32) {
	pageSize := 100
	pageNum := totalcount/pageSize + 1
	return int32(pageSize), int32(pageNum)
}
