package manager

import (
	"fmt"
	openapiv1 "github.com/alibabacloud-go/darabonba-openapi/client"
	ecs20140526 "github.com/alibabacloud-go/ecs-20140526/v2/client"
	"github.com/alibabacloud-go/tea/tea"
)

type (
	EcsClient struct {
		key, secret      string
		region, endpoint string
		*ecs20140526.Client
	}
)

// 获取ecs全量信息 权限AliyunECSReadOnlyAccess
func (m *Manager) DescribeEcsInstances() ([]*ecs20140526.DescribeInstancesResponseBodyInstancesInstance, error) {
	req := new(ecs20140526.DescribeInstancesRequest)
	req.SetRegionId(m.region)
	req.SetPageSize(MaxPageSize)
	req.SetPageNumber(DefaultPageNum)
	resp, err := m.ecs.DescribeInstances(req)
	if err != nil {
		return nil, err
	}
	result := make([]*ecs20140526.DescribeInstancesResponseBodyInstancesInstance, 0, 100)
	result = append(result, resp.Body.Instances.Instance...)
	totalCount := resp.Body.TotalCount
	pageSize, pageNum := pageCaculator(int(*totalCount))
	req.SetPageSize(pageSize)
	var i int32
	for i = 2; i < 2+pageNum; i++ {
		req.SetPageNumber(i)
		resp, err := m.ecs.DescribeInstances(req)
		if err != nil {
			return nil, err
		}
		result = append(result, resp.Body.Instances.Instance...)
	}
	return result, nil
}

func NewEcsClient(key, secret, region string) Option {
	if len(key) == 0 {
		panic("accessKey for ecs is required")
	}
	if len(secret) == 0 {
		panic("accessSecret for ecs is required")
	}
	if len(region) == 0 {
		panic("region for ecs is required")
	}

	return func(m *Manager) error {
		ecsConfig := &openapiv1.Config{
			AccessKeyId:     tea.String(key),
			AccessKeySecret: tea.String(secret),
			RegionId:        tea.String(region), // tea.String("cn-beijing"),
			// Endpoint:        tea.String("tds.aliyuncs.com"),
		}
		ecs, err := ecs20140526.NewClient(ecsConfig)
		if err != nil {
			return err
		}
		m.ecs = &EcsClient{
			key:    key,
			secret: secret,
			region: region,
			Client: ecs,
		}
		return nil
	}
}

func (m *Manager) EcsKey(instanceID string) string {
	return fmt.Sprintf("%s||%s", "acs_ecs_dashboard", instanceID)
}
