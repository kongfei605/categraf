package manager

import (
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	sas20181203 "github.com/alibabacloud-go/sas-20181203/v2/client"
	"github.com/alibabacloud-go/tea/tea"
)

type (
	SasClient struct {
		key, secret      string
		region, endpoint string

		*sas20181203.Client
	}
)

func NewSasClient(key, secret, region, endpoint string) Option {
	if len(key) == 0 {
		panic("accessKey for sas is required")
	}
	if len(secret) == 0 {
		panic("accessSecret for sas is required")
	}
	if len(region) == 0 {
		panic("region for sas is required")
	}
	if len(endpoint) == 0 {
		panic("endpoint for sas is required")
	}
	return func(m *Manager) error {
		sasConfig := &openapi.Config{
			AccessKeyId:     tea.String(key),
			AccessKeySecret: tea.String(secret),
			RegionId:        tea.String(region),   // tea.String("cn-beijing"),
			Endpoint:        tea.String(endpoint), // tea.String("tds.aliyuncs.com"),
		}
		sas, err := sas20181203.NewClient(sasConfig)
		if err != nil {
			return err
		}
		m.sas = &SasClient{
			key:      key,
			secret:   secret,
			region:   region,
			endpoint: endpoint,
			Client:   sas,
		}
		return nil
	}
}

// 云安全中心https://help.aliyun.com/document_detail/475833.html
func (m *Manager) GetResourceDetail(id string) ([]*sas20181203.GetCloudAssetDetailResponseBodyInstances, error) {
	req := new(sas20181203.GetCloudAssetDetailRequest)
	req.SetVendor(0)
	req.SetAssetType(0)
	req.SetAssetSubType(0)
	ins := &sas20181203.GetCloudAssetDetailRequestCloudAssetInstances{}
	ins.SetRegionId(id)
	req.CloudAssetInstances = append(req.CloudAssetInstances, ins)
	resp, err := m.sas.GetCloudAssetDetail(req)
	if err != nil {
		return nil, err
	}
	return resp.Body.Instances, nil
}

func (m *Manager) GetAllResourceInfo() ([]*sas20181203.DescribeCloudCenterInstancesResponseBodyInstances, error) {
	req := new(sas20181203.DescribeCloudCenterInstancesRequest)
	req.SetRegionId(m.region)
	req.SetPageSize(MaxPageSize)
	req.SetCurrentPage(DefaultPageNum)
	resp, err := m.sas.DescribeCloudCenterInstances(req)
	if err != nil {
		return nil, err
	}
	resources := make([]*sas20181203.DescribeCloudCenterInstancesResponseBodyInstances, 0, 100)
	resources = append(resources, resp.Body.Instances...)

	totalCount := *resp.Body.PageInfo.TotalCount
	pageSize, pageNum := pageCaculator(int(totalCount))
	req.SetPageSize(pageSize)
	var i int32
	for i = 2; i < 2+pageNum; i++ {
		req.SetCurrentPage(i)
		resp, err := m.sas.DescribeCloudCenterInstances(req)
		if err != nil {
			return nil, err
		}
		resources = append(resources, resp.Body.Instances...)
	}
	return resources, nil
}

func (m *Manager) GetResourceInfo(uuid string) (*sas20181203.DescribeAssetDetailByUuidResponseBodyAssetDetail, error) {
	req := new(sas20181203.DescribeAssetDetailByUuidRequest)
	req.SetUuid(uuid)
	resp, err := m.sas.DescribeAssetDetailByUuid(req)
	if err != nil {
		return nil, err
	}
	return resp.Body.AssetDetail, nil
}

func (m *Manager) ListInstances() ([]*sas20181203.ListCloudAssetInstancesResponseBodyInstances, error) {
	req := new(sas20181203.ListCloudAssetInstancesRequest)
	resp, err := m.sas.ListCloudAssetInstances(req)
	if err != nil {
		return nil, err
	}
	resources := make([]*sas20181203.ListCloudAssetInstancesResponseBodyInstances, 0, 100)
	resources = append(resources, resp.Body.Instances...)

	totalCount := *resp.Body.PageInfo.TotalCount
	pageSize, pageNum := pageCaculator(int(totalCount))
	req.SetPageSize(pageSize)
	var i int32
	for i = 1; i < 2+pageNum; i++ {
		req.SetCurrentPage(i)
		resp, err := m.sas.ListCloudAssetInstances(req)
		if err != nil {
			return nil, err
		}
		resources = append(resources, resp.Body.Instances...)
	}
	return resp.Body.Instances, nil

}
