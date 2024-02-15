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

package aks

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/app-net-interface/kubernetes-discovery/cluster"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

type fakeManagedClustersClient struct {
	_Get                         func(ctx context.Context, resourceGroupName string, resourceName string, options *armcontainerservice.ManagedClustersClientGetOptions) (armcontainerservice.ManagedClustersClientGetResponse, error)
	_ListClusterAdminCredentials func(ctx context.Context, resourceGroupName string, resourceName string, options *armcontainerservice.ManagedClustersClientListClusterAdminCredentialsOptions) (armcontainerservice.ManagedClustersClientListClusterAdminCredentialsResponse, error)
}

func (f *fakeManagedClustersClient) Get(ctx context.Context, resourceGroupName string, resourceName string, options *armcontainerservice.ManagedClustersClientGetOptions) (armcontainerservice.ManagedClustersClientGetResponse, error) {
	return f._Get(ctx, resourceGroupName, resourceName, options)
}

func (f *fakeManagedClustersClient) ListClusterAdminCredentials(ctx context.Context, resourceGroupName string, resourceName string, options *armcontainerservice.ManagedClustersClientListClusterAdminCredentialsOptions) (armcontainerservice.ManagedClustersClientListClusterAdminCredentialsResponse, error) {
	return f._ListClusterAdminCredentials(ctx, resourceGroupName, resourceName, options)
}

func TestRetrieve(t *testing.T) {
	type testCase struct {
		subsLister     *runtime.Pager[armsubscriptions.ClientListResponse]
		clustersLister *runtime.Pager[armcontainerservice.ManagedClustersClientListResponse]
		client         managedClustersClient
		opts           []cluster.RetrieveOption
		expErr         error
		expRes         *cluster.RetrieveResults
	}

	var resultChan chan cluster.DiscoveredCluster
	retr := &ClustersRetriever{}
	ctx := context.Background()
	a := assert.New(t)
	testErr := fmt.Errorf("unauthorized")

	// counters
	var counters map[string]int
	resetCounters := func() {
		counters = map[string]int{
			"subs":     0,
			"clusters": 0,
		}
	}
	nextLink := func(counter string) *string {
		if counters[counter] == 0 {
			return ptr.To("next")
		}

		return nil
	}

	testDate := time.Now()
	okClusterId := "/subscriptions/test-sub-id/resourceGroups/test-resource-group/providers/Microsoft.ContainerService/managedClusters/ok-cluster-name"
	errClusterId := "/subscriptions/test-sub-id/resourceGroups/test-resource-group/providers/Microsoft.ContainerService/managedClusters/err-cluster-name"
	errCredsClusterId := "/subscriptions/test-sub-id/resourceGroups/test-resource-group/providers/Microsoft.ContainerService/managedClusters/err-creds-cluster-name"
	testOkCluster := &armcontainerservice.ManagedCluster{
		Location: ptr.To("eastus"),
		ID:       &okClusterId,
		Name:     ptr.To("ok-cluster-name"),
		SystemData: &armcontainerservice.SystemData{
			CreatedAt: &testDate,
		},
		Properties: &armcontainerservice.ManagedClusterProperties{
			PowerState: &armcontainerservice.PowerState{
				Code: ptr.To[armcontainerservice.Code](armcontainerservice.CodeRunning),
			},
		},
	}
	testErrCluster := &armcontainerservice.ManagedCluster{
		Location: ptr.To("eastus"),
		ID:       &errClusterId,
		Name:     ptr.To("err-cluster-name"),
	}
	testErrCredsCluster := &armcontainerservice.ManagedCluster{
		Location: ptr.To("eastus"),
		ID:       &errCredsClusterId,
		Name:     ptr.To("err-creds-cluster-name"),
	}

	okKubeConfig := []byte(
		`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: test-certificate-authority
    server: https://example.org
  name: ok-cluster-name
contexts:
- context:
    cluster: ok-cluster-name
    user: admin
    name: admin@ok-cluster-name
current-context: admin@ok-cluster-name
kind: Config
preferences: {}
users:
- name: admin
  user:
    token: test-token
`)

	anotherKubeConfig := []byte(
		`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: another-certificate-authority
    server: https://another-example.org
    name: ok-cluster-name
contexts:
- context:
    cluster: ok-cluster-name
    user: admin
  name: admin@ok-cluster-name
current-context: admin@ok-cluster-name
kind: Config
preferences: {}
users:
- name: admin
  user:
    token: another-test-token
`)

	testCases := []testCase{
		{
			opts: []cluster.RetrieveOption{
				cluster.WithResultChannel(nil),
			},
			expErr: cluster.ErrorInvalidChannel,
		},
		{
			subsLister: func() *runtime.Pager[armsubscriptions.ClientListResponse] {
				handler := runtime.PagingHandler[armsubscriptions.ClientListResponse]{
					More: func(clr armsubscriptions.ClientListResponse) bool {
						return clr.NextLink != nil
					},
					Fetcher: func(ctx context.Context, clr *armsubscriptions.ClientListResponse) (armsubscriptions.ClientListResponse, error) {
						return armsubscriptions.ClientListResponse{}, testErr
					},
				}

				return runtime.NewPager[armsubscriptions.ClientListResponse](handler)
			}(),
			expErr: testErr,
		},
		{
			subsLister: func() *runtime.Pager[armsubscriptions.ClientListResponse] {
				handler := runtime.PagingHandler[armsubscriptions.ClientListResponse]{
					More: func(clr armsubscriptions.ClientListResponse) bool {
						return clr.NextLink != nil
					},
					Fetcher: func(ctx context.Context, clr *armsubscriptions.ClientListResponse) (armsubscriptions.ClientListResponse, error) {
						return armsubscriptions.ClientListResponse{
							SubscriptionListResult: armsubscriptions.SubscriptionListResult{
								NextLink: nextLink("subs"),
								Value: func() []*armsubscriptions.Subscription {
									defer func() {
										counters["subs"]++
									}()
									switch counters["subs"] {
									case 0:
										return []*armsubscriptions.Subscription{
											{
												SubscriptionID: ptr.To("sub-with-err-clusters"),
											},
										}
									default:
										return []*armsubscriptions.Subscription{}
									}
								}(),
							},
						}, nil
					},
				}

				return runtime.NewPager[armsubscriptions.ClientListResponse](handler)
			}(),
			clustersLister: func() *runtime.Pager[armcontainerservice.ManagedClustersClientListResponse] {
				handler := runtime.PagingHandler[armcontainerservice.ManagedClustersClientListResponse]{
					More: func(mcclr armcontainerservice.ManagedClustersClientListResponse) bool {
						return mcclr.NextLink != nil
					},
					Fetcher: func(ctx context.Context, mcclr *armcontainerservice.ManagedClustersClientListResponse) (armcontainerservice.ManagedClustersClientListResponse, error) {
						return armcontainerservice.ManagedClustersClientListResponse{}, testErr
					},
				}

				return runtime.NewPager[armcontainerservice.ManagedClustersClientListResponse](handler)
			}(),
			expRes: &cluster.RetrieveResults{
				Errors: []cluster.RegionError{
					{
						Error: fmt.Errorf("cannot get more clusters: %w", testErr),
					},
				},
			},
		},
		{
			subsLister: func() *runtime.Pager[armsubscriptions.ClientListResponse] {
				handler := runtime.PagingHandler[armsubscriptions.ClientListResponse]{
					More: func(clr armsubscriptions.ClientListResponse) bool {
						return clr.NextLink != nil
					},
					Fetcher: func(ctx context.Context, clr *armsubscriptions.ClientListResponse) (armsubscriptions.ClientListResponse, error) {
						return armsubscriptions.ClientListResponse{
							SubscriptionListResult: armsubscriptions.SubscriptionListResult{
								NextLink: nextLink("subs"),
								Value: func() []*armsubscriptions.Subscription {
									defer func() {
										counters["subs"]++
									}()
									switch counters["subs"] {
									case 0:
										return []*armsubscriptions.Subscription{
											{
												SubscriptionID: ptr.To("sub-with-no-clusters"),
											},
										}
									default:
										return []*armsubscriptions.Subscription{}
									}
								}(),
							},
						}, nil
					},
				}

				return runtime.NewPager[armsubscriptions.ClientListResponse](handler)
			}(),
			clustersLister: func() *runtime.Pager[armcontainerservice.ManagedClustersClientListResponse] {
				handler := runtime.PagingHandler[armcontainerservice.ManagedClustersClientListResponse]{
					More: func(mcclr armcontainerservice.ManagedClustersClientListResponse) bool {
						return mcclr.NextLink != nil
					},
					Fetcher: func(ctx context.Context, mcclr *armcontainerservice.ManagedClustersClientListResponse) (armcontainerservice.ManagedClustersClientListResponse, error) {
						return armcontainerservice.ManagedClustersClientListResponse{
							ManagedClusterListResult: armcontainerservice.ManagedClusterListResult{},
						}, nil
					},
				}

				return runtime.NewPager[armcontainerservice.ManagedClustersClientListResponse](handler)
			}(),
			expRes: &cluster.RetrieveResults{},
		},
		{
			subsLister: func() *runtime.Pager[armsubscriptions.ClientListResponse] {
				handler := runtime.PagingHandler[armsubscriptions.ClientListResponse]{
					More: func(clr armsubscriptions.ClientListResponse) bool {
						return clr.NextLink != nil
					},
					Fetcher: func(ctx context.Context, clr *armsubscriptions.ClientListResponse) (armsubscriptions.ClientListResponse, error) {
						return armsubscriptions.ClientListResponse{
							SubscriptionListResult: armsubscriptions.SubscriptionListResult{
								NextLink: nextLink("subs"),
								Value: func() []*armsubscriptions.Subscription {
									defer func() {
										counters["subs"]++
									}()
									switch counters["subs"] {
									case 0:
										return []*armsubscriptions.Subscription{
											{
												SubscriptionID: ptr.To("sub-with-some-clusters"),
											},
										}
									default:
										return []*armsubscriptions.Subscription{}
									}
								}(),
							},
						}, nil
					},
				}

				return runtime.NewPager[armsubscriptions.ClientListResponse](handler)
			}(),
			clustersLister: func() *runtime.Pager[armcontainerservice.ManagedClustersClientListResponse] {
				handler := runtime.PagingHandler[armcontainerservice.ManagedClustersClientListResponse]{
					More: func(mcclr armcontainerservice.ManagedClustersClientListResponse) bool {
						return mcclr.NextLink != nil
					},
					Fetcher: func(ctx context.Context, mcclr *armcontainerservice.ManagedClustersClientListResponse) (armcontainerservice.ManagedClustersClientListResponse, error) {
						return armcontainerservice.ManagedClustersClientListResponse{
							ManagedClusterListResult: armcontainerservice.ManagedClusterListResult{
								Value: []*armcontainerservice.ManagedCluster{
									testErrCluster,
									testErrCredsCluster,
									testOkCluster,
								},
							},
						}, nil
					},
				}

				return runtime.NewPager[armcontainerservice.ManagedClustersClientListResponse](handler)
			}(),
			client: &fakeManagedClustersClient{
				_Get: func(ctx context.Context, resourceGroupName, resourceName string, options *armcontainerservice.ManagedClustersClientGetOptions) (armcontainerservice.ManagedClustersClientGetResponse, error) {
					a.Equal("test-resource-group", resourceGroupName)
					switch resourceName {
					case "ok-cluster-name":
						return armcontainerservice.ManagedClustersClientGetResponse{
							ManagedCluster: *testOkCluster,
						}, nil
					case "err-cluster-name":
						return armcontainerservice.ManagedClustersClientGetResponse{}, testErr
					case "err-creds-cluster-name":
						return armcontainerservice.ManagedClustersClientGetResponse{
							ManagedCluster: *testErrCredsCluster,
						}, nil
					default:
						a.Fail("unrecognized cluster")
						return armcontainerservice.ManagedClustersClientGetResponse{}, fmt.Errorf("unrecognized cluster")
					}
				},
				_ListClusterAdminCredentials: func(ctx context.Context, resourceGroupName, resourceName string, options *armcontainerservice.ManagedClustersClientListClusterAdminCredentialsOptions) (armcontainerservice.ManagedClustersClientListClusterAdminCredentialsResponse, error) {
					a.Equal("test-resource-group", resourceGroupName)
					switch resourceName {
					case "ok-cluster-name":
						return armcontainerservice.ManagedClustersClientListClusterAdminCredentialsResponse{
							CredentialResults: armcontainerservice.CredentialResults{
								Kubeconfigs: []*armcontainerservice.CredentialResult{
									{
										Name:  ptr.To("admin"),
										Value: okKubeConfig,
									},
									{
										Name:  ptr.To("admin"),
										Value: anotherKubeConfig,
									},
								},
							},
						}, nil
					case "err-cluster-name":
						return armcontainerservice.ManagedClustersClientListClusterAdminCredentialsResponse{}, nil
					case "err-creds-cluster-name":
						return armcontainerservice.ManagedClustersClientListClusterAdminCredentialsResponse{}, testErr
					default:
						a.Fail("unrecognized cluster")
						return armcontainerservice.ManagedClustersClientListClusterAdminCredentialsResponse{}, fmt.Errorf("unrecognized cluster")
					}
				},
			},
			expRes: &cluster.RetrieveResults{
				DiscoveredClusters: []cluster.DiscoveredCluster{
					&AksDiscoveredCluster{
						data: &cluster.Cluster{
							Name:              "ok-cluster-name",
							ID:                okClusterId,
							EndpointAddress:   "https://example.org",
							CaCertificateData: "test-certificate-authority",
							Platform:          cluster.AKS,
							Location:          "eastus",
							CreatedAt:         testDate,
							Status:            cluster.Active,
							OriginalObject:    testOkCluster,
						},
						subscriptionID: "test-sub-id",
						resourceGroup:  "test-resource-group",
					},
				},
				Errors: []cluster.RegionError{
					{
						Error: fmt.Errorf("cannot get cluster: %w", testErr),
					},
					{
						Error: fmt.Errorf("cannot get cluster data: %w", testErr),
					},
				},
			},
		},
	}

	for _, c := range testCases {
		// reset counters
		resetCounters()

		resultChan = make(chan cluster.DiscoveredCluster, 10)
		retr.subsLister = c.subsLister
		retr.clustersLister = c.clustersLister
		retr.managedClustersClient = c.client
		res, err := retr.Retrieve(ctx, append([]cluster.RetrieveOption{
			cluster.WithResultChannel(resultChan),
		},
			c.opts...,
		)...)

		if c.expErr != nil {
			a.ErrorIs(err, c.expErr)
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
