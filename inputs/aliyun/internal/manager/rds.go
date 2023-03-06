package manager

import (
	"fmt"
	"log"

	openapiv1 "github.com/alibabacloud-go/darabonba-openapi/client"
	rdsv2 "github.com/alibabacloud-go/rds-20140815/v2/client"
	"github.com/alibabacloud-go/tea/tea"
)

const (
	rdsNamespace = "acs_rds"
)

type (
	RDSClient struct {
		key, secret      string
		region, endpoint string
		*rdsv2.Client
	}
	RDSCacheUnit struct {
		ID   string            `json:"string"`
		Name string            `json:"name"`
		Tags map[string]string `json:"tags"`
	}
)

func (m *Manager) DescribeRdsInstances() ([]*rdsv2.DescribeDBInstancesResponseBodyItemsDBInstance, error) {
	req := new(rdsv2.DescribeDBInstancesRequest)
	req.RegionId = tea.String(m.region)

	resp, err := m.rds.DescribeDBInstances(req)
	if err != nil {
		return nil, err
	}
	result := make([]*rdsv2.DescribeDBInstancesResponseBodyItemsDBInstance, 0, 100)
	result = append(result, resp.Body.Items.DBInstance...)
	for resp.Body != nil && resp.Body.NextToken != nil {
		if len(*resp.Body.NextToken) == 0 {
			break
		}
		req.NextToken = resp.Body.NextToken
		resp, err = m.rds.DescribeDBInstances(req)
		if err != nil {
			log.Println("E!", err)
			continue
		}
		result = append(result, resp.Body.Items.DBInstance...)
	}
	return result, nil
}

func (m *Manager) GetRDSDetail(id string) ([]*rdsv2.DescribeDBInstanceAttributeResponseBodyItemsDBInstanceAttribute, error) {
	req := new(rdsv2.DescribeDBInstanceAttributeRequest)
	req.SetDBInstanceId(id)

	resp, err := m.rds.DescribeDBInstanceAttribute(req)
	if err != nil {
		return nil, err
	}
	return resp.Body.Items.DBInstanceAttribute, nil
}

func (m *Manager) GetRDSTags(id string) ([]*rdsv2.ListTagResourcesResponseBodyTagResourcesTagResource, error) {
	req := new(rdsv2.ListTagResourcesRequest)
	req.SetRegionId(m.region)
	req.SetResourceType("INSTANCE")
	ids := tea.String(id)
	req.SetResourceId([]*string{ids})
	resp, err := m.rds.ListTagResources(req)
	if err != nil {
		return nil, err
	}
	result := make([]*rdsv2.ListTagResourcesResponseBodyTagResourcesTagResource, 0, 100)
	result = append(result, resp.Body.TagResources.TagResource...)
	for resp.Body != nil && resp.Body.NextToken != nil {
		req.NextToken = resp.Body.NextToken
		resp, err = m.rds.ListTagResources(req)
		if err != nil {
			log.Println("E!", err)
			continue
		}
		result = append(result, resp.Body.TagResources.TagResource...)
	}
	return result, nil
}

func NewRdsClient(key, secret, region string) Option {
	if len(key) == 0 {
		panic("accessKey for rds is required")
	}
	if len(secret) == 0 {
		panic("accessSecret for rds is required")
	}
	if len(region) == 0 {
		panic("region for rds is required")
	}

	return func(m *Manager) error {
		rdsConfig := &openapiv1.Config{
			AccessKeyId:     tea.String(key),
			AccessKeySecret: tea.String(secret),
			RegionId:        tea.String(region),
		}

		rds, err := rdsv2.NewClient(rdsConfig)
		if err != nil {
			return err
		}
		m.rds = &RDSClient{
			key:    key,
			secret: secret,
			region: region,
			Client: rds,
		}
		return nil
	}
}

func (m *Manager) RDSKey(instanceID string) string {
	return fmt.Sprintf("%s||%s", rdsNamespace, instanceID)
}
