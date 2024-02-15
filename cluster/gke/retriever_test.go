// Copyright (c) 2023 Cisco Systems, Inc. and its affiliates
// All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package gke

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"testing"
	"time"

	"cloud.google.com/go/container/apiv1/containerpb"
	cluster "github.com/app-net-interface/kubernetes-discovery/cluster"
	"github.com/app-net-interface/kubernetes-discovery/cluster/token"
	"github.com/googleapis/gax-go"
	"github.com/panjf2000/ants"
	"github.com/stretchr/testify/assert"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

type mockClient struct {
	_listClusters func(ctx context.Context, req *containerpb.ListClustersRequest) (*containerpb.ListClustersResponse, error)
	_getCluster   func(ctx context.Context, req *containerpb.GetClusterRequest) (*containerpb.Cluster, error)
}

func (m *mockClient) ListClusters(ctx context.Context, req *containerpb.ListClustersRequest, opts ...gax.CallOption) (*containerpb.ListClustersResponse, error) {
	return m._listClusters(ctx, req)
}

func (m *mockClient) GetCluster(ctx context.Context, req *containerpb.GetClusterRequest, opts ...gax.CallOption) (*containerpb.Cluster, error) {
	return m._getCluster(ctx, req)
}

func (m *mockClient) Close() error {
	return nil
}

func initTest() (*ClustersRetriever, error) {
	ctx := context.Background()
	const (
		serviceAccount     = "service_account"
		projectID          = "test-project-id"
		privateKey         = "test-private-key"
		privateKeyID       = "test-private-key-id"
		clientEmail        = "test-client-email"
		cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"
		userInfoEmailScope = "https://www.googleapis.com/auth/userinfo.email"
	)
	type gcpServiceAccount struct {
		Type                    string `json:"type,omitempty"`
		ProjectID               string `json:"project_id,omitempty"`
		PrivateKeyID            string `json:"private_key_id,omitempty"`
		PrivateKey              string `json:"private_key,omitempty"`
		ClientEmail             string `json:"client_email,omitempty"`
		ClientID                string `json:"client_id,omitempty"`
		AuthURI                 string `json:"auth_uri,omitempty"`
		TokenURI                string `json:"token_uri,omitempty"`
		AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url,omitempty"`
		ClientX509CertURL       string `json:"client_x509_cert_url,omitempty"`
	}

	saBytes, _ := json.Marshal(gcpServiceAccount{
		Type:         serviceAccount,
		ProjectID:    projectID,
		PrivateKeyID: privateKeyID,
		PrivateKey:   privateKey,
		ClientEmail:  clientEmail,
	})

	creds, err := google.CredentialsFromJSON(ctx, saBytes, cloudPlatformScope, userInfoEmailScope)
	if err != nil {
		return nil, err
	}

	return &ClustersRetriever{
		credentials:   creds,
		clientOptions: []option.ClientOption{},
	}, nil
}

func TestNewClustersRetriever(t *testing.T) {
	retr, err := initTest()
	if err != nil {
		t.Fatal(err)
	}
	testCases := []struct {
		creds  *google.Credentials
		expRes *ClustersRetriever
		expErr error
	}{
		{
			expErr: cluster.ErrorInvalidCredentials,
		},
		{
			creds: retr.credentials,
			expRes: &ClustersRetriever{
				credentials:   retr.credentials,
				clientOptions: []option.ClientOption{option.WithCredentials(retr.credentials)},
			},
		},
	}

	a := assert.New(t)
	for _, c := range testCases {
		res, err := NewClustersRetriever(c.creds)
		a.Equal(c.expErr, err)
		a.Equal(c.expRes, res)
	}
}

func TestRetrieve(t *testing.T) {
	ctx := context.Background()
	retr, err := initTest()
	if err != nil {
		t.Fatal(err)
	}

	var resultChan chan cluster.DiscoveredCluster
	a := assert.New(t)
	type listerFunc func(ctx context.Context, req *containerpb.ListClustersRequest) (*containerpb.ListClustersResponse, error)
	type testCase struct {
		retrOptions []cluster.RetrieveOption
		lister      listerFunc
		expErr      error
		expRes      *cluster.RetrieveResults
	}
	wp, err := ants.NewPool(10)
	if err != nil {
		t.Fatal(err)
	}
	defer wp.Release()
	testTime := time.Date(2023, time.January, 1, 0, 0, 0, 0, time.Local)
	testResults := []*containerpb.Cluster{
		{
			Name:        "my-cluster-1",
			Description: "my-cluster-1-desc",
			Endpoint:    "https://test-endpoint-1.com",
			CreateTime:  testTime.Format(time.RFC3339),
			Status:      containerpb.Cluster_RUNNING,
			Location:    "test-location-1",
			Id:          "test-id-1",
			MasterAuth: &containerpb.MasterAuth{
				ClusterCaCertificate: "test-certificate-1",
			},
		},
		{
			Name:        "my-cluster-2",
			Description: "my-cluster-2-desc",
			Endpoint:    "https://test-endpoint-2.com",
			CreateTime:  testTime.Format(time.RFC3339),
			Status:      containerpb.Cluster_RUNNING,
			Location:    "test-location-2",
			Id:          "test-id-2",
			MasterAuth: &containerpb.MasterAuth{
				ClusterCaCertificate: "test-certificate-2",
			},
		},
	}
	expRes := []*GkeDiscoveredCluster{
		{
			data: &cluster.Cluster{
				Name:              testResults[0].Name,
				ID:                testResults[0].Id,
				EndpointAddress:   testResults[0].Endpoint,
				CaCertificateData: testResults[0].MasterAuth.ClusterCaCertificate,
				Platform:          cluster.GKE,
				Location:          testResults[0].Location,
				CreatedAt:         testTime,
				Status:            cluster.Active,
				OriginalObject:    testResults[0],
			},
			tokenAuth: token.NewGcpTokenGenerator(retr.credentials),
		},
		{
			data: &cluster.Cluster{
				Name:              testResults[1].Name,
				ID:                testResults[1].Id,
				EndpointAddress:   testResults[1].Endpoint,
				CaCertificateData: testResults[1].MasterAuth.ClusterCaCertificate,
				Platform:          cluster.GKE,
				Location:          testResults[1].Location,
				CreatedAt:         testTime,
				Status:            cluster.Active,
				OriginalObject:    testResults[1],
			},
			tokenAuth: token.NewGcpTokenGenerator(retr.credentials),
		},
	}
	testUnauthorized := fmt.Errorf("unauthorized")
	testCases := []testCase{
		{
			retrOptions: []cluster.RetrieveOption{
				cluster.WithResultChannel(nil),
			},
			expErr: cluster.ErrorInvalidChannel,
		},
		{
			retrOptions: []cluster.RetrieveOption{
				cluster.WithRegions("test-location-1", "test-location-2", "test-location-3", "test-location-4"),
			},
			lister: func(ctx context.Context, req *containerpb.ListClustersRequest) (*containerpb.ListClustersResponse, error) {
				a.NotNil(req)
				switch req.Parent {
				case path.Join("projects", retr.credentials.ProjectID, "locations", "test-location-1"):
					return &containerpb.ListClustersResponse{
						Clusters: []*containerpb.Cluster{
							testResults[0],
						},
					}, nil
				case path.Join("projects", retr.credentials.ProjectID, "locations", "test-location-2"):
					return &containerpb.ListClustersResponse{
						Clusters: []*containerpb.Cluster{
							testResults[1],
						},
					}, nil
				case path.Join("projects", retr.credentials.ProjectID, "locations", "test-location-3"):
					return &containerpb.ListClustersResponse{
						Clusters: []*containerpb.Cluster{},
					}, nil
				case path.Join("projects", retr.credentials.ProjectID, "locations", "test-location-4"):
					return &containerpb.ListClustersResponse{
						Clusters: []*containerpb.Cluster{},
					}, testUnauthorized
				default:
					a.Fail("invalid data sent")
					return &containerpb.ListClustersResponse{
						Clusters: []*containerpb.Cluster{},
					}, testUnauthorized
				}
			},
			expRes: &cluster.RetrieveResults{
				DiscoveredClusters: []cluster.DiscoveredCluster{
					expRes[0], expRes[1],
				},
				Errors: []cluster.RegionError{
					{
						Region: "test-location-4",
						Error:  testUnauthorized,
					},
				},
			},
		},
		{
			retrOptions: []cluster.RetrieveOption{},
			lister: func(ctx context.Context, req *containerpb.ListClustersRequest) (*containerpb.ListClustersResponse, error) {
				a.NotNil(req)
				a.Equal(path.Join("projects", retr.credentials.ProjectID, "locations", "-"), req.Parent)

				return &containerpb.ListClustersResponse{
					Clusters: []*containerpb.Cluster{
						testResults[0], testResults[1],
					},
				}, nil
			},
			expRes: &cluster.RetrieveResults{
				DiscoveredClusters: []cluster.DiscoveredCluster{
					expRes[0], expRes[1],
				},
				Errors: []cluster.RegionError{},
			},
		},
		{
			retrOptions: []cluster.RetrieveOption{
				cluster.WithWorkerPool(wp),
				cluster.WithMaxWorkers(2),
			},
			lister: func(ctx context.Context, req *containerpb.ListClustersRequest) (*containerpb.ListClustersResponse, error) {
				a.NotNil(req)
				a.Equal(path.Join("projects", retr.credentials.ProjectID, "locations", "-"), req.Parent)

				return &containerpb.ListClustersResponse{
					Clusters: []*containerpb.Cluster{},
				}, testUnauthorized
			},
			expRes: &cluster.RetrieveResults{
				DiscoveredClusters: []cluster.DiscoveredCluster{},
				Errors: []cluster.RegionError{
					{
						Region: "",
						Error:  testUnauthorized,
					},
				},
			},
		},
	}

	for _, c := range testCases {
		resultChan = make(chan cluster.DiscoveredCluster, 10)
		client := &mockClient{
			_listClusters: c.lister,
		}
		retr.client = client
		res, err := retr.Retrieve(ctx, append([]cluster.RetrieveOption{
			cluster.WithResultChannel(resultChan),
		},
			c.retrOptions...,
		)...)

		if c.expErr != nil {
			a.Equal(c.expErr, err)
			continue
		}
		a.NoError(err)
		results := []cluster.DiscoveredCluster{}

		i := 0
		for result := range resultChan {
			results = append(results, result)
			i++
		}

		a.ElementsMatch(c.expRes.DiscoveredClusters, res.DiscoveredClusters)
		a.ElementsMatch(c.expRes.Errors, res.Errors)
		a.ElementsMatch(c.expRes.DiscoveredClusters, results)

	}
}

func TestGetCluster(t *testing.T) {
	ctx := context.Background()
	retr, err := initTest()
	if err != nil {
		t.Fatal(err)
	}

	a := assert.New(t)
	type getterFunc func(ctx context.Context, req *containerpb.GetClusterRequest) (*containerpb.Cluster, error)
	type testCase struct {
		region string
		name   string
		getter getterFunc
		expErr error
		expRes cluster.DiscoveredCluster
	}
	testClusterName := "my-cluster"
	testRegionName := "my-region"
	unauthorized := fmt.Errorf("unauthorized")
	testTime := time.Date(2023, time.January, 1, 0, 0, 0, 0, time.Local)
	testCluster := &containerpb.Cluster{
		Name:        testClusterName,
		Description: "my-cluster-1-desc",
		Endpoint:    "https://test-endpoint-1.com",
		CreateTime:  testTime.Format(time.RFC3339),
		Status:      containerpb.Cluster_STOPPING,
		Location:    testRegionName,
		Id:          "test-id-1",
		MasterAuth: &containerpb.MasterAuth{
			ClusterCaCertificate: "test-certificate-1",
		},
	}
	testResult := &GkeDiscoveredCluster{
		data: &cluster.Cluster{
			Name:              testCluster.Name,
			ID:                testCluster.Id,
			EndpointAddress:   testCluster.Endpoint,
			CaCertificateData: testCluster.MasterAuth.ClusterCaCertificate,
			Platform:          cluster.GKE,
			Location:          testCluster.Location,
			CreatedAt:         testTime,
			Status:            cluster.Stopping,
			OriginalObject:    testCluster,
		},
		tokenAuth: token.NewGcpTokenGenerator(retr.credentials),
	}
	testCases := []testCase{
		{
			expErr: cluster.ErrorInvalidClusterName,
		},
		{
			name:   testClusterName,
			expErr: cluster.ErrorInvalidRegionName,
		},
		{
			name:   testClusterName,
			region: testRegionName,
			getter: func(ctx context.Context, req *containerpb.GetClusterRequest) (*containerpb.Cluster, error) {
				return &containerpb.Cluster{}, unauthorized
			},
			expErr: unauthorized,
		},
		{
			name:   testClusterName,
			region: testRegionName,
			getter: func(ctx context.Context, req *containerpb.GetClusterRequest) (*containerpb.Cluster, error) {
				return testCluster, nil
			},
			expRes: testResult,
		},
	}

	for _, c := range testCases {
		retr.client = &mockClient{_getCluster: c.getter}

		res, err := retr.GetCluster(ctx, c.region, c.name)
		a.Equal(c.expErr, err)
		a.Equal(c.expRes, res)
	}
}
