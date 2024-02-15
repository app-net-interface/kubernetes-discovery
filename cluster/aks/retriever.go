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
	"errors"
	"fmt"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/app-net-interface/kubernetes-discovery/cluster"
	"github.com/panjf2000/ants"
	kc "github.com/volvo-cars/lingon/pkg/kubeconfig"
)

type managedClustersClient interface {
	Get(ctx context.Context, resourceGroupName string, resourceName string, options *armcontainerservice.ManagedClustersClientGetOptions) (armcontainerservice.ManagedClustersClientGetResponse, error)
	ListClusterAdminCredentials(ctx context.Context, resourceGroupName string, resourceName string, options *armcontainerservice.ManagedClustersClientListClusterAdminCredentialsOptions) (armcontainerservice.ManagedClustersClientListClusterAdminCredentialsResponse, error)
}

type ClustersRetriever struct {
	identity azcore.TokenCredential

	// For testing
	subsLister            *runtime.Pager[armsubscriptions.ClientListResponse]
	clustersLister        *runtime.Pager[armcontainerservice.ManagedClustersClientListResponse]
	managedClustersClient managedClustersClient
}

func NewClustersRetriever(identity azcore.TokenCredential) (*ClustersRetriever, error) {
	if identity == nil {
		return nil, cluster.ErrorNoCredentialsProvided
	}

	return &ClustersRetriever{
		identity: identity,
	}, nil
}

func (r *ClustersRetriever) Retrieve(ctx context.Context, opts ...cluster.RetrieveOption) (*cluster.RetrieveResults, error) {
	// -------------------------------------
	// Parse options
	// -------------------------------------

	options := &cluster.RetrieveOptions{}
	for _, opt := range opts {
		if err := opt(options); err != nil {
			return nil, err
		}
	}

	if options.MaxWorkers == 0 {
		options.MaxWorkers = 100
	}

	pool := options.WorkerPool
	if options.WorkerPool == nil {
		wp, err := ants.NewPool(int(options.MaxWorkers))
		if err != nil {
			return nil, errors.Join(cluster.ErrorCreatingPool, err)
		}

		pool = wp
	}

	// -------------------------------------
	// Init data
	// -------------------------------------

	chanResult := make(chan *result, 256)
	resp := &cluster.RetrieveResults{
		DiscoveredClusters: []cluster.DiscoveredCluster{},
		Errors:             []cluster.RegionError{},
	}
	wait := 0

	defer func() {
		close(chanResult)

		// Release the pool, but only if this is *our* pool, not user's.
		if options.WorkerPool == nil {
			pool.Release()
		}

		// Close the channels or send completion.
		if options.ClustersChan != nil && !options.KeepChannelAlive {
			close(options.ClustersChan)
		}
	}()

	// -------------------------------------
	// Start tasks
	// -------------------------------------

	pager, err := func() (*runtime.Pager[armsubscriptions.ClientListResponse], error) {
		if r.subsLister == nil {
			subClient, err := armsubscriptions.NewClientFactory(r.identity, nil)
			if err != nil {
				return nil, fmt.Errorf("cannot get subscriptions client: %w", err)
			}

			return subClient.NewClient().NewListPager(nil), nil
		}

		return r.subsLister, nil
	}()
	if err != nil {
		return nil, err
	}

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("cannot get more subscriptions: %w", err)
		}

		for _, val := range page.Value {
			wait++
			err := pool.Submit(func(subID string) func() {
				return func() {
					r.getAllClustersInSubscription(ctx, subID, pool, chanResult)
				}
			}(*val.SubscriptionID))
			if err != nil {
				wait--
				resp.Errors = append(resp.Errors, cluster.RegionError{
					Error: errors.Join(cluster.ErrorCannotSubmitTask, err),
				})
			}
		}
	}

	// -------------------------------------
	// Wait for results (and send them)
	// -------------------------------------

	for wait > 0 {
		select {
		case <-ctx.Done():
			// At this point all the workers must have received the context
			// cancellation as well, so we will wait for them to send a done
			// signal.
		case res := <-chanResult:
			switch {

			case res.err != nil:
				resp.Errors = append(resp.Errors, cluster.RegionError{
					Error: res.err,
				})

			case res.done:
				wait--

			case res.data != nil:
				if options.ClustersChan != nil {
					options.ClustersChan <- res.data
				}

				resp.DiscoveredClusters = append(resp.DiscoveredClusters, res.data)
			}
		}
	}

	return resp, nil
}

func (r *ClustersRetriever) getAllClustersInSubscription(ctx context.Context, subID string, pool *ants.Pool, chanResult chan<- *result) {
	wg := sync.WaitGroup{}
	defer func() {
		wg.Wait()
		chanResult <- &result{
			sub:  subID,
			done: true,
		}
	}()

	pager, err := func() (*runtime.Pager[armcontainerservice.ManagedClustersClientListResponse], error) {
		if r.clustersLister == nil {
			clientFactory, err := armcontainerservice.NewClientFactory(subID, r.identity, nil)
			if err != nil {
				return nil, fmt.Errorf("cannot get client factory: %w", err)
			}

			return clientFactory.NewManagedClustersClient().NewListPager(nil), nil
		}

		return r.clustersLister, nil
	}()
	if err != nil {
		chanResult <- &result{
			sub: subID,
			err: err,
		}
	}

	for pager.More() {
		clusterPage, err := pager.NextPage(ctx)
		if err != nil {
			chanResult <- &result{
				sub: subID,
				err: fmt.Errorf("cannot get more clusters: %w", err),
			}
			return
		}

		for _, cl := range clusterPage.Value {
			parsedResource, err := arm.ParseResourceID(*cl.ID)
			if err != nil {
				chanResult <- &result{
					sub: subID,
					err: err,
				}

				continue
			}
			resourceGroup := parsedResource.ResourceGroupName

			wg.Add(1)
			err = pool.Submit(func(clusterData *armcontainerservice.ManagedCluster) func() {
				return func() {
					defer wg.Done()

					resp, err := r.GetCluster(ctx, *clusterData.Location, *clusterData.ID)
					if err != nil {
						chanResult <- &result{
							sub:      subID,
							resGroup: resourceGroup,
							err:      err,
						}

						return
					}

					chanResult <- &result{
						data:     resp.(*AksDiscoveredCluster),
						sub:      subID,
						resGroup: resourceGroup,
					}

				}
			}(cl))
			if err != nil {
				wg.Done()
				chanResult <- &result{
					sub:      subID,
					resGroup: resourceGroup,
					err:      errors.Join(cluster.ErrorCannotSubmitTask, err),
				}
			}
		}
	}
}

type result struct {
	data     *AksDiscoveredCluster
	done     bool
	sub      string
	resGroup string
	err      error
}

func (r *ClustersRetriever) GetCluster(ctx context.Context, region, name string) (cluster.DiscoveredCluster, error) {
	parsedResource, err := arm.ParseResourceID(name)
	if err != nil {
		return nil, fmt.Errorf("cannot get resource data: %w", err)
	}

	subscriptionID := parsedResource.SubscriptionID
	resourceGroup := parsedResource.ResourceGroupName
	clusterName := parsedResource.Name

	mclient, err := func() (managedClustersClient, error) {
		if r.managedClustersClient == nil {
			clientFactory, err := armcontainerservice.NewClientFactory(subscriptionID, r.identity, nil)
			if err != nil {
				return nil, fmt.Errorf("cannot get client factory: %w", err)
			}

			return clientFactory.NewManagedClustersClient(), nil
		}

		return r.managedClustersClient, nil
	}()
	if err != nil {
		return nil, err
	}

	resp, err := mclient.Get(ctx, resourceGroup, clusterName, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot get cluster: %w", err)
	}

	admCreds, err := mclient.ListClusterAdminCredentials(ctx, resourceGroup, clusterName, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot get cluster data: %w", err)
	}

	if len(admCreds.Kubeconfigs) == 0 {
		return nil, fmt.Errorf("no cluster authentication found: %w", err)
	}

	k := kc.New()
	if err := k.Unmarshal(admCreds.Kubeconfigs[0].Value); err != nil {
		return nil, fmt.Errorf("no cluster authentication found: %w", err)
	}

	return &AksDiscoveredCluster{
		data:           buildClusterData(&resp.ManagedCluster, k),
		identity:       r.identity,
		subscriptionID: subscriptionID,
		resourceGroup:  resourceGroup,
	}, nil
}
