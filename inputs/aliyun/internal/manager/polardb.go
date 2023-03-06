package manager

import (
	"fmt"

	openapiv1 "github.com/alibabacloud-go/darabonba-openapi/client"
	polardb "github.com/alibabacloud-go/polardb-20170801/v2/client"
	"github.com/alibabacloud-go/tea/tea"
)

const (
	polardbNamespace = "acs_polardb"
)

type (
	PolarDBClient struct {
		key, secret      string
		endpoint, region string
		*polardb.Client
	}
)

func (m *Manager) DescribePolarCluster() ([]*polardb.DescribeDBClustersResponseBodyItemsDBCluster, error) {
	req := new(polardb.DescribeDBClustersRequest)
	req.SetRegionId(m.region)
	req.SetPageSize(100)
	req.SetPageNumber(DefaultPageNum)
	resp, err := m.polar.DescribeDBClusters(req)
	if err != nil {
		return nil, err
	}
	result := make([]*polardb.DescribeDBClustersResponseBodyItemsDBCluster, 0, 100)
	result = append(result, resp.Body.Items.DBCluster...)
	totalCount := resp.Body.TotalRecordCount
	pageSize, pageNum := dbPageCaculator(int(*totalCount))
	req.SetPageSize(pageSize)
	var i int32
	for i = 2; i < 2+pageNum; i++ {
		req.SetPageNumber(i)
		resp, err := m.polar.DescribeDBClusters(req)
		if err != nil {
			return nil, err
		}
		result = append(result, resp.Body.Items.DBCluster...)
	}
	return result, nil
}

func (m *Manager) DescribeDBClusterAttribute(clusterid string) (*polardb.DescribeDBClusterAttributeResponseBody, error) {
	req := new(polardb.DescribeDBClusterAttributeRequest)
	req.SetDBClusterId(clusterid)
	resp, err := m.polar.DescribeDBClusterAttribute(req)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func NewPolarDBClient(key, secret, region string) Option {
	if len(key) == 0 {
		panic("accessKey for polardb is required")
	}
	if len(secret) == 0 {
		panic("accessSecret for polardb is required")
	}
	if len(region) == 0 {
		panic("region for polardb is required")
	}

	return func(m *Manager) error {
		rdsConfig := &openapiv1.Config{
			AccessKeyId:     tea.String(key),
			AccessKeySecret: tea.String(secret),
			RegionId:        tea.String(region),
		}

		polar, err := polardb.NewClient(rdsConfig)
		if err != nil {
			return err
		}
		m.polar = &PolarDBClient{
			key:    key,
			secret: secret,
			region: region,
			Client: polar,
		}
		return nil
	}
}

func (m *Manager) PolarDBKey(instance string) string {
	return fmt.Sprintf("%s||%s", polardbNamespace, instance)
}

func dbPageCaculator(count int) (int32, int32) {
	pageSize := 100
	pageNum := count/pageSize + 1
	return int32(pageSize), int32(pageNum)
}
